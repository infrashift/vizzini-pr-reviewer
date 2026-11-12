// Package reviewer is Vizzini's public Go API: describe, review and improve a
// unified diff with an LLM behind a LiteLLM proxy. Every model answer is
// validated against the embedded CUE schema, and anchored to the diff, before
// it is returned (ADR-0003, ADR-0007). An Engine is stateless and safe for
// concurrent use.
package reviewer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/internal/analysis"
	"github.com/infrashift/vizzini-pr-reviewer/internal/chunker"
	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/llm"
	"github.com/infrashift/vizzini-pr-reviewer/internal/okf"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// ErrNoChanges is returned by Describe when nothing reviewable remains after
// excluded paths are dropped.
var ErrNoChanges = errors.New("reviewer: no reviewable changes in the patch")

const defaultConcurrency = 4

// Engine runs reviews. Create it with New.
type Engine struct {
	llm         *llm.Client
	validator   *schema.Validator
	chunker     *chunker.Chunker
	risk        *schema.RiskPolicy
	floor       *schema.NoiseFloor
	exclude     []string
	concurrency int
	maxKnowTok  int
	maxCtxTok   int
}

const defaultMaxKnowledgeTokens = 2000

// chunkContext is the extra, non-diff material sent with one chunk.
type chunkContext struct {
	lint      []string
	knowledge []llm.KnowledgeDoc
	context   []llm.ContextSnippet
}

// New validates cfg, loads the tokenizer and compiles the schema.
func New(cfg Config) (*Engine, error) { //nolint:gocritic // hugeParam: one-shot constructor; by-value Config keeps call sites simple
	client, err := llm.New(llm.Config{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		MaxRetries: cfg.MaxRetries,
		MaxRepairs: cfg.MaxRepairs,
		Backoff:    cfg.Backoff,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	ch, err := chunker.New(chunker.Options{MaxChunkTokens: cfg.MaxChunkTokens, Encoding: cfg.Encoding})
	if err != nil {
		return nil, err
	}
	v, err := schema.New()
	if err != nil {
		return nil, err
	}
	risk, err := v.RiskPolicy(cfg.RiskPolicy)
	if err != nil {
		return nil, fmt.Errorf("risk policy: %w", err)
	}
	if verr := analysis.ValidateRiskPatterns(risk); verr != nil {
		return nil, fmt.Errorf("risk policy: %w", verr)
	}
	floor, err := v.NoiseFloor(cfg.NoiseFloor)
	if err != nil {
		return nil, fmt.Errorf("noise floor: %w", err)
	}
	conc := cfg.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	know := cfg.MaxKnowledgeTokens
	if know <= 0 {
		know = defaultMaxKnowledgeTokens
	}
	ctxTok := cfg.MaxContextTokens
	if ctxTok <= 0 {
		ctxTok = defaultMaxContextTokens
	}
	return &Engine{llm: client, validator: v, chunker: ch, risk: risk, floor: floor, exclude: cfg.Exclude, concurrency: conc, maxKnowTok: know, maxCtxTok: ctxTok}, nil
}

// Policy returns a severity gate validated by the #PolicyGate schema. A nil
// failOn selects the schema default (Critical and Major); an empty, non-nil
// slice blocks nothing. Unknown severities are rejected.
func (e *Engine) Policy(failOn []Severity) (*PolicyGate, error) {
	return e.validator.Policy(failOn)
}

func (e *Engine) parse(patch io.Reader) (*diff.Diff, error) {
	return diff.Parse(patch, diff.Options{Exclude: e.exclude})
}

// Review asks the model for findings on every chunk of the patch and merges
// the answers. A patch with nothing reviewable returns an empty review
// without calling the model.
func (e *Engine) Review(ctx context.Context, patch io.Reader, opts ReviewOptions) (*ReviewOutput, error) { //nolint:gocritic // hugeParam: public API; options by value keep embedders' call sites simple and safe
	ctx, attr, meter, finish := begin(ctx, "review")
	defer finish()
	d, err := e.parse(patch)
	if err != nil {
		return nil, err
	}
	var diags []analysis.Diagnostic
	for i, rep := range opts.LintReports {
		ds, lerr := analysis.ParseLintReport(rep, opts.LintRoot)
		if lerr != nil {
			return nil, fmt.Errorf("lint report %d: %w", i+1, lerr)
		}
		diags = append(diags, ds...)
	}
	risk := analysis.ScoreRisk(d.Files, e.risk)
	out := &ReviewOutput{Findings: []Finding{}, Excluded: d.Excluded, BaseRiskScore: risk.Score, RiskScore: risk.Score, RiskFiles: risk.Files,
		Attribution: attr}
	defer func() {
		u := meter.Usage()
		out.Usage = &u
	}()

	var chunks []chunker.Chunk
	for _, c := range e.chunker.Chunk(d.Files) {
		if c.Oversized && opts.SkipOversized {
			out.Skipped = append(out.Skipped, c.Parts[0].File.Path)
			continue
		}
		chunks = append(chunks, c)
	}
	out.Chunks = len(chunks)
	if len(chunks) == 0 {
		out.Summary = "No reviewable changes."
		return out, nil
	}

	var bundle *okf.Bundle
	if opts.Knowledge != nil {
		if bundle, err = okf.Load(opts.Knowledge); err != nil {
			return nil, err
		}
	}
	cctx := make([]chunkContext, len(chunks))
	used := map[string]bool{}
	for i := range chunks {
		cctx[i].lint = chunkLint(diags, &chunks[i])
		out.Diagnostics += len(cctx[i].lint)
		cctx[i].knowledge = e.chunkKnowledge(bundle, &chunks[i])
		for _, k := range cctx[i].knowledge {
			used[k.ID] = true
		}
	}
	for id := range used {
		out.KnowledgeDocs = append(out.KnowledgeDocs, id)
	}
	slices.Sort(out.KnowledgeDocs)
	if opts.Repo != nil {
		ctxs, cerr := e.repoContext(opts.Repo, chunks)
		if cerr != nil {
			return nil, cerr
		}
		for i := range chunks {
			cctx[i].context = ctxs[i]
			out.ContextSnippets += len(ctxs[i])
		}
	}

	results := make([]*schema.ReviewOutput, len(chunks))
	if err := e.forEach(ctx, len(chunks), func(ctx context.Context, i int) error {
		r, err := e.reviewChunk(llm.WithMeta(ctx, "vizzini.chunk", fmt.Sprintf("%d/%d", i+1, len(chunks))), &chunks[i], i+1, len(chunks), &cctx[i])
		results[i] = r
		return err
	}); err != nil {
		return nil, err
	}

	summaries := make([]string, 0, len(results))
	for i, r := range results {
		out.ModelRiskScore = max(out.ModelRiskScore, r.RiskScore)
		out.Findings = append(out.Findings, r.Findings...)
		if len(results) == 1 {
			summaries = append(summaries, r.Summary)
		} else {
			summaries = append(summaries, fmt.Sprintf("Part %d of %d: %s", i+1, len(results), r.Summary))
		}
	}
	out.Summary = strings.Join(summaries, "\n\n")
	out.RiskScore = max(out.BaseRiskScore, out.ModelRiskScore)
	out.Findings, out.MergedFindings = foldDuplicates(out.Findings)
	// Auto-approval sees every finding, suppressed or filtered or not: a
	// suppression rule or the noise floor decides what is posted, never that
	// a human may be skipped.
	out.AutoApprove = e.autoApprove(out)
	out.Findings, out.Suppressed = opts.Suppressions.Apply(out.Findings, time.Now())
	if !opts.AllFindings {
		out.Findings, out.Filtered = e.belowFloor(out.Findings)
	}
	slices.SortStableFunc(out.Findings, func(a, b Finding) int { return compareFindings(&a, &b) })
	slices.SortStableFunc(out.Filtered, func(a, b Finding) int { return compareFindings(&a, &b) })
	return out, nil
}

// compareFindings orders findings most severe first, then by place.
func compareFindings(a, b *Finding) int {
	return cmp.Or(
		cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)),
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.StartLine, b.StartLine),
		cmp.Compare(a.Title, b.Title),
	)
}

// belowFloor splits findings into those the noise floor keeps and those it
// filters (ISSUE-032).
func (e *Engine) belowFloor(findings []Finding) (kept, filtered []Finding) {
	kept = make([]Finding, 0, len(findings))
	for i := range findings {
		if e.floor.Matches(findings[i].Severity, findings[i].Category) {
			filtered = append(filtered, findings[i])
		} else {
			kept = append(kept, findings[i])
		}
	}
	return kept, filtered
}

// autoApprove applies the Milestone 4.4 rule. It is deliberately strict: a
// change with nothing reviewed (e.g. only lockfiles) or with skipped hunks
// always needs a human.
func (e *Engine) autoApprove(out *ReviewOutput) bool {
	if out.Chunks == 0 || len(out.Skipped) > 0 || out.RiskScore > e.risk.AutoApproveThreshold {
		return false
	}
	for i := range out.RiskFiles {
		if e.risk.IsSensitive(out.RiskFiles[i].Weight) {
			return false
		}
	}
	for i := range out.Findings {
		if s := out.Findings[i].Severity; s == SeverityCritical || s == SeverityMajor {
			return false
		}
	}
	return true
}

// Describe writes a pull request description. The model sees every changed
// file's header and as much content as fits in one chunk.
func (e *Engine) Describe(ctx context.Context, patch io.Reader) (*PRDescription, error) {
	ctx, _, _, finish := begin(ctx, "describe")
	defer finish()
	d, err := e.parse(patch)
	if err != nil {
		return nil, err
	}
	if len(d.Files) == 0 {
		return nil, ErrNoChanges
	}
	chunks := e.chunker.Chunk(d.Files)
	headers := make([]string, 0, len(d.Files))
	known := make(map[string]bool, len(d.Files))
	for _, f := range d.Files {
		headers = append(headers, strings.TrimSuffix(f.Header(), "\n"))
		known[f.Path] = true
	}
	p, err := llm.Render("describe", llm.DescribeData{Files: headers, Diff: chunks[0].Render(), Truncated: len(chunks) > 1})
	if err != nil {
		return nil, err
	}

	var out *PRDescription
	if _, err := e.llm.CompleteJSON(ctx, p, func(b []byte) error {
		r, verr := e.validator.ValidateDescription(b)
		if verr != nil {
			return verr
		}
		for j, c := range r.Changes {
			if !known[c.File] {
				return fmt.Errorf("%w: changes.%d: file %q is not part of this diff", schema.ErrInvalid, j, c.File)
			}
		}
		out = r
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// Improve asks the model for concrete code suggestions on every chunk, sorted
// by file and line. Oversized hunks are always included.
func (e *Engine) Improve(ctx context.Context, patch io.Reader) ([]CodeSuggestion, error) {
	ctx, _, _, finish := begin(ctx, "improve")
	defer finish()
	d, err := e.parse(patch)
	if err != nil {
		return nil, err
	}
	chunks := e.chunker.Chunk(d.Files)
	results := make([][]CodeSuggestion, len(chunks))
	if err := e.forEach(ctx, len(chunks), func(ctx context.Context, i int) error {
		s, err := e.improveChunk(ctx, &chunks[i], i+1, len(chunks))
		results[i] = s
		return err
	}); err != nil {
		return nil, err
	}

	out := []CodeSuggestion{}
	for _, r := range results {
		out = append(out, r...)
	}
	slices.SortStableFunc(out, func(a, b CodeSuggestion) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.StartLine, b.StartLine))
	})
	return out, nil
}

// reviewChunk sends one chunk for review and returns the first answer that
// passes the schema and whose findings are anchored to this chunk.
func (e *Engine) reviewChunk(ctx context.Context, c *chunker.Chunk, part, parts int, cc *chunkContext) (*schema.ReviewOutput, error) {
	p, err := llm.Render("review", llm.ReviewData{Diff: c.Render(), Part: part, Parts: parts, Lint: cc.lint, Knowledge: cc.knowledge, Context: cc.context})
	if err != nil {
		return nil, err
	}
	a := newAnchors(c.Parts)
	cited := make(map[string]bool, len(cc.knowledge))
	for _, k := range cc.knowledge {
		cited[k.ID] = true
	}
	var out *schema.ReviewOutput
	_, err = e.llm.CompleteJSON(ctx, p, func(b []byte) error {
		r, verr := e.validator.ValidateReview(b)
		if verr != nil {
			return verr
		}
		for j := range r.Findings {
			f := &r.Findings[j]
			if aerr := a.check("findings", j, f.File, f.StartLine, f.EndLine); aerr != nil {
				return aerr
			}
			if cerr := checkCitations(j, f.Citations, cited); cerr != nil {
				return cerr
			}
		}
		out = r
		return nil
	})
	return out, err
}

// checkCitations requires every cited id to be a document this chunk was
// given, the knowledge-base counterpart of line anchoring (ADR-0007).
func checkCitations(idx int, citations []string, allowed map[string]bool) error {
	for _, c := range citations {
		if allowed[c] {
			continue
		}
		if len(allowed) == 0 {
			return fmt.Errorf("%w: findings.%d: cites %q, but no knowledge documents were provided; remove citations and do not use category Architecture", schema.ErrInvalid, idx, c)
		}
		ids := make([]string, 0, len(allowed))
		for id := range allowed {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		return fmt.Errorf("%w: findings.%d: cites %q, which is not a provided document; cite one of: %s", schema.ErrInvalid, idx, c, strings.Join(ids, ", "))
	}
	return nil
}

// chunkKnowledge selects the bundle documents relevant to this chunk's
// files, strongest match first, within the knowledge token budget. A first
// document larger than the whole budget is truncated rather than dropped.
func (e *Engine) chunkKnowledge(b *okf.Bundle, c *chunker.Chunk) []llm.KnowledgeDoc {
	if b == nil {
		return nil
	}
	files := make([]*diff.File, 0, len(c.Parts))
	for _, p := range c.Parts {
		files = append(files, &diff.File{Path: p.File.Path, Source: p.File.Source, Hunks: p.Hunks})
	}
	var out []llm.KnowledgeDoc
	budget := e.maxKnowTok
	for _, m := range b.Match(okf.TargetsFor(files)) {
		k := llm.KnowledgeDoc{ID: m.Doc.ID, Title: m.Doc.Title, Body: m.Doc.Body}
		cost := e.chunker.CountTokens(k.Title + "\n" + k.Body)
		if cost > budget {
			if len(out) > 0 {
				break
			}
			k.Body = e.truncateTokens(k.Body, budget-e.chunker.CountTokens(k.Title)-8) + "\n[…truncated]"
			cost = budget
		}
		out = append(out, k)
		budget -= cost
	}
	return out
}

// truncateTokens keeps whole lines of s while they fit in n tokens.
func (e *Engine) truncateTokens(s string, n int) string {
	var b strings.Builder
	used := 0
	for _, line := range strings.SplitAfter(s, "\n") {
		t := e.chunker.CountTokens(line)
		if used+t > n {
			break
		}
		b.WriteString(line)
		used += t
	}
	return strings.TrimRight(b.String(), "\n")
}

// chunkLint returns the diagnostics on lines this chunk adds, formatted for
// the prompt. A split file contributes only the hunks in this chunk.
func chunkLint(diags []analysis.Diagnostic, c *chunker.Chunk) []string {
	if len(diags) == 0 {
		return nil
	}
	files := make([]*diff.File, 0, len(c.Parts))
	for _, p := range c.Parts {
		files = append(files, &diff.File{Path: p.File.Path, Source: p.File.Source, Hunks: p.Hunks})
	}
	matched := analysis.OnChangedLines(diags, files)
	out := make([]string, 0, len(matched))
	for _, d := range matched {
		out = append(out, d.String())
	}
	return out
}

// improveChunk is reviewChunk for improvement suggestions.
func (e *Engine) improveChunk(ctx context.Context, c *chunker.Chunk, part, parts int) ([]CodeSuggestion, error) {
	p, err := llm.Render("improve", llm.ImproveData{Diff: c.Render(), Part: part, Parts: parts})
	if err != nil {
		return nil, err
	}
	a := newAnchors(c.Parts)
	var out []CodeSuggestion
	_, err = e.llm.CompleteJSON(ctx, p, func(b []byte) error {
		r, verr := e.validator.ValidateImprove(b)
		if verr != nil {
			return verr
		}
		for j := range r.Suggestions {
			s := &r.Suggestions[j]
			if aerr := a.check("suggestions", j, s.File, s.StartLine, s.EndLine); aerr != nil {
				return aerr
			}
		}
		out = r.Suggestions
		return nil
	})
	return out, err
}

// forEach runs fn for indexes 0..n-1 with at most e.concurrency in flight.
// The first error cancels the remaining calls and is returned.
func (e *Engine) forEach(ctx context.Context, n int, fn func(context.Context, int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		sem      = make(chan struct{}, e.concurrency)
	)
	for i := range n {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(ctx, i); err != nil {
				once.Do(func() {
					firstErr = fmt.Errorf("chunk %d of %d: %w", i+1, n, err)
					cancel()
				})
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func severityRank(s Severity) int {
	if i := slices.Index(schema.Severities, s); i >= 0 {
		return i
	}
	return len(schema.Severities)
}

// anchors holds the new-file line ranges each file shows in one chunk, so a
// finding can be checked against lines the model actually saw (ADR-0007).
type anchors map[string][][2]int

func newAnchors(parts []chunker.Part) anchors {
	a := anchors{}
	for _, p := range parts {
		f := p.File
		if f.Source.IsDelete || f.Source.IsBinary || len(f.Hunks) == 0 {
			// No new-side lines to anchor to; any line is accepted.
			if _, ok := a[f.Path]; !ok {
				a[f.Path] = nil
			}
			continue
		}
		for _, h := range p.Hunks {
			start := int(h.Fragment.NewPosition)
			end := start + max(int(h.Fragment.NewLines), 1) - 1
			a[f.Path] = append(a[f.Path], [2]int{start, end})
		}
	}
	return a
}

func (a anchors) check(field string, idx int, file string, start, end int) error {
	ranges, ok := a[file]
	if !ok {
		return fmt.Errorf("%w: %s.%d: file %q is not part of this diff; use a path exactly as shown after ###", schema.ErrInvalid, field, idx, file)
	}
	if ranges == nil {
		return nil
	}
	for _, r := range ranges {
		if start <= r[1] && end >= r[0] {
			return nil
		}
	}
	shown := make([]string, 0, len(ranges))
	for _, r := range ranges {
		shown = append(shown, fmt.Sprintf("%d-%d", r[0], r[1]))
	}
	return fmt.Errorf("%w: %s.%d: lines %d-%d of %s are outside the changed hunks (new lines %s)",
		schema.ErrInvalid, field, idx, start, end, file, strings.Join(shown, ", "))
}
