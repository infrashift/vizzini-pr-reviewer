// Package risk is Vizzini's deterministic, LLM-free risk score as a public
// API (ISSUE-015). It computes the same base_risk_score and risk_files that a
// review reports, from the diff and a #RiskPolicy alone, so an embedder such
// as mrman can score every change in a list without a model, a proxy or a
// tokenizer.
//
// The package is a dependency leaf by contract: it must never link the LLM
// client, the tokenizer or the knowledge-base loader. A test enforces this.
package risk

import (
	"fmt"
	"io"
	"sync"

	"github.com/infrashift/vizzini-pr-reviewer/internal/analysis"
	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// Policy is a validated #RiskPolicy: path weights, scale, and the
// auto-approve, sensitive and high thresholds. Build one with LoadPolicy or
// DefaultPolicy so the schema defaults and bounds apply.
type Policy = schema.RiskPolicy

// Weight is one path-weight rule of a Policy. The first matching pattern wins.
type Weight = schema.RiskWeight

// Band is a coarse reading of a score: low, medium or high.
type Band = schema.RiskBand

// Bands, lowest first.
const (
	BandLow    = schema.RiskBandLow
	BandMedium = schema.RiskBandMedium
	BandHigh   = schema.RiskBandHigh
)

// FileChange is one changed file reduced to what the score reads: its path,
// the added and deleted lines' text (without the +/- prefix), and whether it
// is binary. Callers with their own diff model fill it directly.
type FileChange = analysis.FileChange

// FileRisk is one file's contribution to the score. Its JSON is the
// risk_files entry of a review report.
type FileRisk = analysis.FileRisk

// Assessment is the deterministic score of a whole change: Score equals a
// review's base_risk_score and Files its risk_files.
type Assessment = analysis.Risk

// RepoPolicyPath is where a repository keeps its own risk policy, relative to
// its root. pr-review takes the file with --risk-policy; embedders look for
// it here.
const RepoPolicyPath = ".vizzini/risk.json"

var validator = sync.OnceValues(schema.New)

// LoadPolicy validates a JSON policy against #RiskPolicy and checks its
// globs. Omitted fields take the schema defaults; nil or empty data selects
// every default.
func LoadPolicy(data []byte) (*Policy, error) {
	v, err := validator()
	if err != nil {
		return nil, err
	}
	p, err := v.RiskPolicy(data)
	if err != nil {
		return nil, fmt.Errorf("risk policy: %w", err)
	}
	if err := analysis.ValidateRiskPatterns(p); err != nil {
		return nil, fmt.Errorf("risk policy: %w", err)
	}
	return p, nil
}

// DefaultPolicy returns a fresh copy of the schema's default policy. It
// panics only if the embedded schema itself is broken, which tests rule out.
func DefaultPolicy() *Policy {
	p, err := LoadPolicy(nil)
	if err != nil {
		panic("risk: embedded default policy: " + err.Error())
	}
	return p
}

// ScoreFile computes one file's contribution under p. Unlike Score it keeps
// no ordering, so callers can align results with their own file list, which
// may name one path twice (a patch series touching it in two patches).
func ScoreFile(f *FileChange, p *Policy) FileRisk {
	return analysis.ScoreFile(f, p)
}

// Combine turns per-file contributions into the 0..100 score:
// 100 × (1 − e^(−total points / scale)), rounded.
func Combine(files []FileRisk, p *Policy) int {
	return analysis.CombineRisk(files, p)
}

// Score assesses a set of changed files: every file's contribution, the
// combined score and its band. Files are sorted highest contribution first.
func Score(files []FileChange, p *Policy) Assessment {
	return analysis.ScoreChanges(files, p)
}

// AssessPatch reads a unified diff and assesses it exactly as a review
// would. exclude holds glob patterns for paths to drop, with the same
// meaning as reviewer.Config.Exclude: nil drops the default generated and
// vendored paths, and an empty, non-nil slice keeps everything.
func AssessPatch(r io.Reader, p *Policy, exclude []string) (Assessment, error) {
	d, err := diff.Parse(r, diff.Options{Exclude: exclude})
	if err != nil {
		return Assessment{}, err
	}
	return analysis.ScoreRisk(d.Files, p), nil
}
