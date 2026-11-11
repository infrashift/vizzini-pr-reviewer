package analysis

import (
	"cmp"
	"go/scanner"
	"go/token"
	"math"
	"slices"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// complexityPoints is how many changed lines one added decision point is
// worth: new branching is riskier than new straight-line code.
const complexityPoints = 5

// binaryPoints stands in for the unseen content of a changed binary file.
const binaryPoints = 10

// FileRisk explains one file's contribution to the risk score.
type FileRisk struct {
	Path string `json:"path"`
	// Weight is the path weight from the policy (first matching pattern).
	Weight float64 `json:"weight"`
	// Changed counts added plus deleted lines.
	Changed int `json:"changed"`
	// ComplexityDelta is decision points added minus removed (Go files:
	// if, for, case, &&, ||). Negative values do not lower the score.
	ComplexityDelta int `json:"complexity_delta"`
	// Points is Weight × (Changed + 5 × max(0, ComplexityDelta)).
	Points float64 `json:"points"`
}

// Risk is a deterministic blast-radius estimate for a diff.
type Risk struct {
	// Score is 100 × (1 − e^(−total points / scale)), rounded, in 0..100.
	Score int `json:"score"`
	// Files lists each file's contribution, highest first.
	Files []FileRisk `json:"files"`
}

// ScoreRisk computes the deterministic risk score of files under policy p.
// It depends only on the diff and the policy, never on the model.
func ScoreRisk(files []*diff.File, p *schema.RiskPolicy) Risk {
	r := Risk{Files: make([]FileRisk, 0, len(files))}
	total := 0.0
	for _, f := range files {
		fr := FileRisk{Path: f.Path, Weight: weightFor(f.Path, p)}
		for i := range f.Hunks {
			for _, l := range f.Hunks[i].Lines {
				switch l.Op {
				case gitdiff.OpAdd:
					fr.Changed++
					fr.ComplexityDelta += decisionPoints(f.Path, l.Text)
				case gitdiff.OpDelete:
					fr.Changed++
					fr.ComplexityDelta -= decisionPoints(f.Path, l.Text)
				case gitdiff.OpContext:
				}
			}
		}
		raw := float64(fr.Changed + complexityPoints*max(0, fr.ComplexityDelta))
		if f.Source.IsBinary {
			raw += binaryPoints
		}
		fr.Points = round2(fr.Weight * raw)
		total += fr.Points
		r.Files = append(r.Files, fr)
	}
	r.Score = int(math.Round(100 * (1 - math.Exp(-total/p.Scale))))
	r.Score = min(max(r.Score, 0), 100)
	slices.SortStableFunc(r.Files, func(a, b FileRisk) int {
		return cmp.Or(cmp.Compare(b.Points, a.Points), cmp.Compare(a.Path, b.Path))
	})
	return r
}

func weightFor(p string, policy *schema.RiskPolicy) float64 {
	for _, w := range policy.Weights {
		// Patterns are validated by the caller (ValidateRiskPatterns).
		if ok, _ := diff.MatchGlob(w.Pattern, p); ok {
			return w.Weight
		}
	}
	return policy.DefaultWeight
}

// ValidateRiskPatterns reports the first malformed glob in the policy.
func ValidateRiskPatterns(p *schema.RiskPolicy) error {
	for _, w := range p.Weights {
		if _, err := diff.MatchGlob(w.Pattern, "x"); err != nil {
			return err
		}
	}
	return nil
}

// decisionPoints counts McCabe decision points on one line of Go code.
// Other languages contribute 0: their complexity is not estimated.
func decisionPoints(path, line string) int {
	if !strings.HasSuffix(path, ".go") {
		return 0
	}
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(line))
	s.Init(file, []byte(line), nil, 0)
	n := 0
	for {
		_, tok, _ := s.Scan()
		switch tok { //nolint:exhaustive // only decision tokens matter
		case token.EOF:
			return n
		case token.IF, token.FOR, token.CASE, token.LAND, token.LOR:
			n++
		}
	}
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
