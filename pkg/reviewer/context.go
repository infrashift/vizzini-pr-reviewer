package reviewer

import (
	"cmp"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/infrashift/vizzini-pr-reviewer/internal/chunker"
	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/llm"
	"github.com/infrashift/vizzini-pr-reviewer/internal/repoctx"
)

const (
	defaultMaxContextTokens = 1500
	maxDefinitionsPerSymbol = 2
	maxCallersPerSymbol     = 3
)

// chunkWant is what one chunk's changed lines use and define.
type chunkWant struct {
	used, defined map[repoctx.Family]map[string]bool
}

func wantFor(c *chunker.Chunk) chunkWant {
	w := chunkWant{used: map[repoctx.Family]map[string]bool{}, defined: map[repoctx.Family]map[string]bool{}}
	for _, p := range c.Parts {
		var added, deleted []string
		for i := range p.Hunks {
			for _, l := range p.Hunks[i].Lines {
				switch l.Op {
				case gitdiff.OpAdd:
					added = append(added, l.Text)
				case gitdiff.OpDelete:
					deleted = append(deleted, l.Text)
				case gitdiff.OpContext:
				}
			}
		}
		fam, used, defined := repoctx.Symbols(p.File.Path, added, deleted)
		if fam == "" {
			continue
		}
		for _, n := range used {
			addName(w.used, fam, n)
		}
		for _, n := range defined {
			addName(w.defined, fam, n)
		}
	}
	return w
}

func addName(m map[repoctx.Family]map[string]bool, fam repoctx.Family, n string) {
	if m[fam] == nil {
		m[fam] = map[string]bool{}
	}
	m[fam][n] = true
}

// repoContext scans repo once for every chunk's symbols and returns each
// chunk's snippets (ISSUE-023, ADR-0014).
func (e *Engine) repoContext(repo fs.FS, chunks []chunker.Chunk) ([][]llm.ContextSnippet, error) {
	wants := make([]chunkWant, len(chunks))
	all := repoctx.Want{Used: map[repoctx.Family]map[string]bool{}, Defined: map[repoctx.Family]map[string]bool{}}
	for i := range chunks {
		wants[i] = wantFor(&chunks[i])
		for fam, names := range wants[i].used {
			for n := range names {
				addName(all.Used, fam, n)
			}
		}
		for fam, names := range wants[i].defined {
			for n := range names {
				addName(all.Defined, fam, n)
			}
		}
	}
	out := make([][]llm.ContextSnippet, len(chunks))
	if len(all.Used) == 0 && len(all.Defined) == 0 {
		return out, nil
	}
	exclude := e.exclude
	if exclude == nil {
		exclude = diff.DefaultExcludes
	}
	found, err := repoctx.Scan(repo, all, repoctx.Limits{
		Exclude: exclude,
		MatchGlob: func(g, p string) bool {
			ok, _ := diff.MatchGlob(g, p)
			return ok
		},
	})
	if err != nil {
		return nil, fmt.Errorf("repository context: %w", err)
	}
	for i := range chunks {
		out[i] = e.selectContext(&chunks[i], wants[i], found)
	}
	return out, nil
}

// selectContext picks a chunk's snippets: definitions of what it uses, then
// callers of what it defines; nearest files first; never a region the chunk
// already shows; within the token budget.
func (e *Engine) selectContext(c *chunker.Chunk, w chunkWant, found []repoctx.Snippet) []llm.ContextSnippet {
	shown := newAnchors(c.Parts)
	dirs := make([]string, 0, len(c.Parts))
	for _, p := range c.Parts {
		dirs = append(dirs, path.Dir(p.File.Path))
	}
	var cands []repoctx.Snippet
	for _, s := range found {
		wanted := w.used
		if s.Kind == "caller" {
			wanted = w.defined
		}
		if !wanted[s.Family][s.Symbol] || overlapsShown(shown, &s) {
			continue
		}
		cands = append(cands, s)
	}
	slices.SortStableFunc(cands, func(a, b repoctx.Snippet) int {
		return cmp.Or(
			cmp.Compare(kindRank(a.Kind), kindRank(b.Kind)),
			cmp.Compare(proximity(dirs, a.Path), proximity(dirs, b.Path)),
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Start, b.Start),
		)
	})
	perSymbol := map[string]int{}
	budget := e.maxCtxTok
	var out []llm.ContextSnippet
	for _, s := range cands {
		key := s.Kind + "\x00" + s.Symbol
		limit := maxDefinitionsPerSymbol
		if s.Kind == "caller" {
			limit = maxCallersPerSymbol
		}
		if perSymbol[key] >= limit {
			continue
		}
		cs := llm.ContextSnippet{Path: s.Path, Start: s.Start, End: s.End, Kind: s.Kind, Symbol: s.Symbol, Text: numbered(s.Text, s.Start)}
		cost := e.chunker.CountTokens(cs.Path + " " + cs.Kind + " " + cs.Symbol + "\n" + cs.Text)
		if cost > budget {
			continue
		}
		budget -= cost
		perSymbol[key]++
		out = append(out, cs)
	}
	return out
}

func kindRank(k string) int {
	if k == "definition" {
		return 0
	}
	return 1
}

// proximity ranks a path by how close it is to the chunk's files: the same
// directory, the same top-level directory, or elsewhere.
func proximity(dirs []string, p string) int {
	d := path.Dir(p)
	if slices.Contains(dirs, d) {
		return 0
	}
	top := func(s string) string { return strings.SplitN(s, "/", 2)[0] }
	if slices.ContainsFunc(dirs, func(x string) bool { return top(x) == top(d) }) {
		return 1
	}
	return 2
}

func overlapsShown(shown anchors, s *repoctx.Snippet) bool {
	for _, r := range shown[s.Path] {
		if s.Start <= r[1] && s.End >= r[0] {
			return true
		}
	}
	return false
}

// numbered prefixes each line with its file line number, like the diff the
// model already reads.
func numbered(text string, start int) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = fmt.Sprintf("%d %s", start+i, l)
	}
	return strings.Join(lines, "\n")
}
