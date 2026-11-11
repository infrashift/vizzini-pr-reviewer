// Package analysis holds deterministic, non-LLM analysis that feeds the
// review: linter diagnostics today; dependency ordering and risk scoring as
// later milestones land.
package analysis

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

// Diagnostic is one linter issue.
type Diagnostic struct {
	Linter string
	Text   string
	File   string
	Line   int
	Column int
}

// String formats the diagnostic as "file:line:col linter: text" for prompts.
func (d Diagnostic) String() string {
	if d.Column > 0 {
		return fmt.Sprintf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Linter, d.Text)
	}
	return fmt.Sprintf("%s:%d %s: %s", d.File, d.Line, d.Linter, d.Text)
}

// golangciReport is the subset of golangci-lint's JSON output we read. The
// shape is the same for v1 (--out-format=json) and v2
// (--output.json.path=stdout).
type golangciReport struct {
	Issues []struct {
		FromLinter string
		Text       string
		Pos        struct {
			Filename string
			Line     int
			Column   int
		}
	}
}

// ParseGolangciJSON reads a golangci-lint JSON report. Paths are normalized to
// slash-separated, cleaned, repository-relative form; run golangci-lint from
// the repository root so they match the diff.
func ParseGolangciJSON(r io.Reader) ([]Diagnostic, error) {
	var rep golangciReport
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return nil, fmt.Errorf("parse golangci-lint JSON: %w", err)
	}
	out := make([]Diagnostic, 0, len(rep.Issues))
	for _, is := range rep.Issues {
		if is.Pos.Filename == "" || is.Pos.Line < 1 {
			continue
		}
		out = append(out, Diagnostic{
			Linter: is.FromLinter,
			Text:   strings.TrimSpace(is.Text),
			File:   normalize(is.Pos.Filename),
			Line:   is.Pos.Line,
			Column: is.Pos.Column,
		})
	}
	return out, nil
}

func normalize(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.TrimPrefix(path.Clean(p), "./")
}

// OnChangedLines keeps the diagnostics that sit on a line the diff added or
// modified (a "+" line), sorted by file, line and column. Diagnostics on
// context lines, outside the hunks, or in files the diff does not touch are
// dropped: the review is about the change, not the whole file.
func OnChangedLines(diags []Diagnostic, files []*diff.File) []Diagnostic {
	added := map[string]map[int]bool{}
	for _, f := range files {
		for i := range f.Hunks {
			for _, l := range f.Hunks[i].Lines {
				if l.Op != gitdiff.OpAdd {
					continue
				}
				if added[f.Path] == nil {
					added[f.Path] = map[int]bool{}
				}
				added[f.Path][int(l.NewLine)] = true
			}
		}
	}
	var out []Diagnostic
	for _, d := range diags {
		if added[d.File][d.Line] {
			out = append(out, d)
		}
	}
	slices.SortStableFunc(out, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
	})
	return out
}
