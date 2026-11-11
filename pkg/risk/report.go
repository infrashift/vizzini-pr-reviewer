package risk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// APIVersion identifies Vizzini's JSON reports (ADR-0010). pkg/reviewer
// re-exports it, so the two can never disagree.
const APIVersion = "vizzini.infrashift.io/v1"

// ErrNotReviewReport marks input that is not a Vizzini review: another
// report kind, another API version, or JSON missing the review's risk fields.
var ErrNotReviewReport = errors.New("risk: not a Vizzini review report")

// Report is the risk part of a finished review: what an embedder that never
// calls a model reads from a review someone else ran (in CI, or by hand with
// pr-review).
type Report struct {
	// RiskScore is max(BaseRiskScore, ModelRiskScore) (ADR-0008).
	RiskScore int `json:"risk_score"`
	// BaseRiskScore is the deterministic score. Comparing it with a fresh
	// Assessment of the same diff tells whether the report is stale.
	BaseRiskScore int `json:"base_risk_score"`
	// ModelRiskScore is the model's own estimate.
	ModelRiskScore int `json:"model_risk_score"`
	// RiskFiles explains BaseRiskScore per file, highest first.
	RiskFiles []FileRisk `json:"risk_files,omitempty"`
	// AutoApprove is the review's verdict. Treat it as a suggestion: it
	// never replaces a human's decision to approve.
	AutoApprove bool `json:"auto_approve"`
	// Summary is the model's summary of the change.
	Summary string `json:"summary"`
	// Findings are the review's findings in report order, never nil. Their
	// text is model output: render it as untrusted.
	Findings []Finding `json:"findings"`
	// KnowledgeDocs lists the knowledge-base document IDs the model was
	// given; findings cite from these.
	KnowledgeDocs []string `json:"knowledge_docs,omitempty"`
}

// Finding is one review finding, in the JSON shape pkg/reviewer.Finding
// writes. Severity and Category stay strings so this package needs neither
// the CUE schema nor a model; pkg/reviewer's tests keep the shapes in step.
type Finding struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	// Severity is Critical, Major, Minor or Trivial.
	Severity string `json:"severity"`
	// Category is Security, Performance, Logic, Maintainability or Architecture.
	Category   string   `json:"category"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Suggestion string   `json:"suggestion,omitempty"`
	Citations  []string `json:"citations,omitempty"`
}

// reviewFields is a review as it arrives, with pointers to tell absent
// fields from zero ones.
type reviewFields struct {
	APIVersion     *string         `json:"apiVersion"`
	Kind           *string         `json:"kind"`
	RiskScore      *int            `json:"risk_score"`
	BaseRiskScore  *int            `json:"base_risk_score"`
	ModelRiskScore *int            `json:"model_risk_score"`
	RiskFiles      []FileRisk      `json:"risk_files"`
	AutoApprove    *bool           `json:"auto_approve"`
	Summary        string          `json:"summary"`
	Findings       json.RawMessage `json:"findings"`
	KnowledgeDocs  []string        `json:"knowledge_docs"`
}

// ParseReport reads the risk fields of a review. It accepts both shapes a
// review comes in: the report pr-review --format json prints (apiVersion
// vizzini.infrashift.io/v1, kind Review, ADR-0010), and a bare ReviewOutput
// marshaled from Go. Fields it does not need are ignored, so reports from
// newer Vizzini versions still parse. It rejects other kinds and versions,
// out-of-range scores, and a risk_score that is not max(base, model).
func ParseReport(data []byte) (*Report, error) {
	var f reviewFields
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotReviewReport, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after the JSON object", ErrNotReviewReport)
	}
	if f.APIVersion != nil && *f.APIVersion != APIVersion {
		return nil, fmt.Errorf("%w: apiVersion %q, want %q", ErrNotReviewReport, *f.APIVersion, APIVersion)
	}
	if f.Kind != nil && *f.Kind != "Review" {
		return nil, fmt.Errorf("%w: kind %q, want Review", ErrNotReviewReport, *f.Kind)
	}
	if (f.APIVersion == nil) != (f.Kind == nil) {
		return nil, fmt.Errorf("%w: apiVersion and kind go together", ErrNotReviewReport)
	}
	for name, present := range map[string]bool{
		"risk_score":       f.RiskScore != nil,
		"base_risk_score":  f.BaseRiskScore != nil,
		"model_risk_score": f.ModelRiskScore != nil,
		"auto_approve":     f.AutoApprove != nil,
		"findings":         len(f.Findings) > 0 && !bytes.Equal(f.Findings, []byte("null")),
	} {
		if !present {
			return nil, fmt.Errorf("%w: missing %s", ErrNotReviewReport, name)
		}
	}
	r := &Report{
		RiskScore:      *f.RiskScore,
		BaseRiskScore:  *f.BaseRiskScore,
		ModelRiskScore: *f.ModelRiskScore,
		RiskFiles:      f.RiskFiles,
		AutoApprove:    *f.AutoApprove,
		Summary:        f.Summary,
		KnowledgeDocs:  f.KnowledgeDocs,
	}
	if err := json.Unmarshal(f.Findings, &r.Findings); err != nil {
		return nil, fmt.Errorf("%w: findings: %w", ErrNotReviewReport, err)
	}
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	for i := range r.Findings {
		if err := r.Findings[i].check(); err != nil {
			return nil, fmt.Errorf("%w: findings[%d]: %w", ErrNotReviewReport, i, err)
		}
	}
	for name, v := range map[string]int{"risk_score": r.RiskScore, "base_risk_score": r.BaseRiskScore, "model_risk_score": r.ModelRiskScore} {
		if v < 0 || v > 100 {
			return nil, fmt.Errorf("%w: %s %d outside 0..100", ErrNotReviewReport, name, v)
		}
	}
	if r.RiskScore != max(r.BaseRiskScore, r.ModelRiskScore) {
		return nil, fmt.Errorf("%w: risk_score %d is not max(base %d, model %d)", ErrNotReviewReport, r.RiskScore, r.BaseRiskScore, r.ModelRiskScore)
	}
	for i := range r.RiskFiles {
		if r.RiskFiles[i].Path == "" {
			return nil, fmt.Errorf("%w: risk_files[%d] has no path", ErrNotReviewReport, i)
		}
	}
	return r, nil
}

// check enforces what an embedder relies on to anchor and show a finding.
// Vizzini validated the full schema before writing the report; this guards
// against hand-edited or truncated files.
func (f *Finding) check() error {
	switch {
	case f.File == "":
		return errors.New("no file")
	case f.StartLine < 1:
		return fmt.Errorf("start_line %d < 1", f.StartLine)
	case f.EndLine < f.StartLine:
		return fmt.Errorf("end_line %d before start_line %d", f.EndLine, f.StartLine)
	case f.Severity == "":
		return errors.New("no severity")
	case f.Title == "":
		return errors.New("no title")
	}
	return nil
}

// Divergence is how far the model's score sits above the deterministic one:
// ModelRiskScore − BaseRiskScore, or 0 when the model is not higher. A large
// value means the model saw risk that size and path weights cannot.
func (r *Report) Divergence() int {
	return max(0, r.ModelRiskScore-r.BaseRiskScore)
}
