package reviewer

import (
	"cmp"
	"slices"
	"strings"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// foldDuplicates merges findings that report one problem twice: once by its
// nature (say Security) and once as the knowledge-base decision it violates
// (Architecture). Models do this unprompted; ISSUE-019 / ADR-0012.
//
// An Architecture finding folds into a partner when both are in the same
// file, their line ranges overlap, the partner is not itself Architecture,
// and they cite at least one document in common. The shared citation is what
// ties the two to one decision: a different problem on the same line (an
// unsafe return next to an eval) does not cite it, so it stays separate.
//
// The merged finding is Architecture, so garmr's VZ-002 still sees the
// violated decision. It takes the more severe severity, the partner's title
// (the specific problem), both bodies, the union of the spans, the partner's
// suggestion (or the Architecture one's), and the union of the citations.
// Pairing is one-to-one and independent of input order: findings are put in a
// total order first, and an Architecture finding folds into its most severe
// partner. It returns the findings and how many pairs were folded.
func foldDuplicates(in []Finding) ([]Finding, int) {
	fs := slices.Clone(in)
	slices.SortStableFunc(fs, func(a, b Finding) int { return totalOrder(&a, &b) })
	gone := make([]bool, len(fs))
	folded := make([]bool, len(fs))
	n := 0
	for i := range fs {
		a := &fs[i]
		if gone[i] || folded[i] || a.Category != schema.CategoryArchitecture || len(a.Citations) == 0 {
			continue
		}
		for j := range fs {
			b := &fs[j]
			if j == i || gone[j] || folded[j] || b.Category == schema.CategoryArchitecture || !sameProblem(a, b) {
				continue
			}
			*b = fold(b, a)
			folded[j], gone[i] = true, true
			n++
			break
		}
	}
	out := make([]Finding, 0, len(fs)-n)
	for i := range fs {
		if !gone[i] {
			out = append(out, fs[i])
		}
	}
	return out, n
}

// sameProblem reports whether Architecture finding a and finding b describe
// one problem: same file, overlapping lines, a shared citation.
func sameProblem(a, b *Finding) bool {
	if a.File != b.File || a.StartLine > b.EndLine || b.StartLine > a.EndLine {
		return false
	}
	for _, c := range a.Citations {
		if slices.Contains(b.Citations, c) {
			return true
		}
	}
	return false
}

// fold merges Architecture finding arch into partner p.
func fold(p, arch *Finding) Finding {
	m := *p
	m.Category = schema.CategoryArchitecture
	if severityRank(arch.Severity) < severityRank(p.Severity) {
		m.Severity = arch.Severity
	}
	m.StartLine = min(p.StartLine, arch.StartLine)
	m.EndLine = max(p.EndLine, arch.EndLine)
	m.Body = strings.TrimSpace(p.Body) + "\n\nViolates a documented design decision: " + strings.TrimSpace(arch.Body)
	if m.Suggestion == "" {
		m.Suggestion = arch.Suggestion
	}
	m.Citations = slices.Concat(p.Citations, arch.Citations)
	slices.Sort(m.Citations)
	m.Citations = slices.Compact(m.Citations)
	return m
}

// totalOrder sorts findings most severe first and breaks every tie, so the
// result never depends on the order the model or the chunks produced them.
func totalOrder(a, b *Finding) int {
	return cmp.Or(
		cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)),
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.StartLine, b.StartLine),
		cmp.Compare(a.EndLine, b.EndLine),
		cmp.Compare(a.Category, b.Category),
		cmp.Compare(a.Title, b.Title),
		cmp.Compare(a.Body, b.Body),
		cmp.Compare(a.Suggestion, b.Suggestion),
		slices.Compare(a.Citations, b.Citations),
	)
}
