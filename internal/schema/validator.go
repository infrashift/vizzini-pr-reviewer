// Package schema validates raw LLM JSON against the embedded CUE schema and
// decodes it into Go structs. Nothing reaches callers without passing through
// here (CLAUDE.md rule 3, ADR-0003).
package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"

	cueschema "github.com/infrashift/vizzini-pr-reviewer/schema"
)

// ErrInvalid marks data that does not satisfy the schema. Callers can test for
// it with errors.Is to decide whether to ask the model for a corrected answer.
var ErrInvalid = errors.New("schema validation failed")

// Severity ranks a finding. Values mirror #Severity in schema/review.cue.
type Severity string

// Severities, most to least severe.
const (
	SeverityCritical Severity = "Critical"
	SeverityMajor    Severity = "Major"
	SeverityMinor    Severity = "Minor"
	SeverityTrivial  Severity = "Trivial"
)

// Severities lists every severity, most severe first.
var Severities = []Severity{SeverityCritical, SeverityMajor, SeverityMinor, SeverityTrivial}

// Category classifies a finding. Values mirror #Category in schema/review.cue.
type Category string

// Categories.
const (
	CategorySecurity        Category = "Security"
	CategoryPerformance     Category = "Performance"
	CategoryLogic           Category = "Logic"
	CategoryMaintainability Category = "Maintainability"
	// CategoryArchitecture marks a violated design decision; such findings
	// must cite the knowledge-base document they rely on.
	CategoryArchitecture Category = "Architecture"
)

// Categories lists every category.
var Categories = []Category{CategorySecurity, CategoryPerformance, CategoryLogic, CategoryMaintainability, CategoryArchitecture}

// Finding mirrors #Finding.
type Finding struct {
	File       string   `json:"file"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	Severity   Severity `json:"severity"`
	Category   Category `json:"category"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Suggestion string   `json:"suggestion,omitempty"`
	Citations  []string `json:"citations,omitempty"`
}

// ReviewOutput mirrors #ReviewOutput.
type ReviewOutput struct {
	Summary   string    `json:"summary"`
	RiskScore int       `json:"risk_score"`
	Findings  []Finding `json:"findings"`
}

// PRType classifies a pull request. Values mirror #PRType.
type PRType string

// PR types.
const (
	PRTypeFeature  PRType = "Feature"
	PRTypeBugfix   PRType = "Bugfix"
	PRTypeRefactor PRType = "Refactor"
	PRTypeDocs     PRType = "Docs"
	PRTypeTests    PRType = "Tests"
	PRTypeChore    PRType = "Chore"
)

// PRTypes lists every PR type.
var PRTypes = []PRType{PRTypeFeature, PRTypeBugfix, PRTypeRefactor, PRTypeDocs, PRTypeTests, PRTypeChore}

// FileChange mirrors #FileChange.
type FileChange struct {
	File        string `json:"file"`
	Description string `json:"description"`
}

// PRDescription mirrors #PRDescription.
type PRDescription struct {
	Title   string       `json:"title"`
	Type    PRType       `json:"type"`
	Summary string       `json:"summary"`
	Changes []FileChange `json:"changes"`
}

// CodeSuggestion mirrors #CodeSuggestion.
type CodeSuggestion struct {
	File         string   `json:"file"`
	StartLine    int      `json:"start_line"`
	EndLine      int      `json:"end_line"`
	Category     Category `json:"category"`
	Summary      string   `json:"summary"`
	ExistingCode string   `json:"existing_code"`
	ImprovedCode string   `json:"improved_code"`
}

// Explanation mirrors #Explanation.
type Explanation struct {
	Answer string `json:"answer"`
}

// AffectedFile mirrors #AffectedFile.
type AffectedFile struct {
	Path   string `json:"path"`
	Change string `json:"change"`
	Reason string `json:"reason"`
}

// PlanStep mirrors #PlanStep.
type PlanStep struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
}

// PlanPhase mirrors #PlanPhase.
type PlanPhase struct {
	Name  string     `json:"name"`
	Steps []PlanStep `json:"steps"`
}

// ImplementationPlan mirrors #ImplementationPlan.
type ImplementationPlan struct {
	Summary       string         `json:"summary"`
	AffectedFiles []AffectedFile `json:"affected_files"`
	Phases        []PlanPhase    `json:"phases"`
	Verification  []string       `json:"verification"`
	Risks         []string       `json:"risks,omitempty"`
	References    []string       `json:"references,omitempty"`
}

// ImproveOutput mirrors #ImproveOutput.
type ImproveOutput struct {
	Suggestions []CodeSuggestion `json:"suggestions"`
}

// Validator holds the compiled schema. It is safe for concurrent use: CUE's
// runtime is mutated during unification, so calls are serialized (ADR-0003).
type Validator struct {
	mu     sync.Mutex
	ctx    *cue.Context
	schema cue.Value
}

// New compiles the embedded schema.
func New() (*Validator, error) {
	ctx := cuecontext.New()
	v := ctx.CompileString(cueschema.ReviewCUE, cue.Filename("review.cue"))
	if err := v.Err(); err != nil {
		return nil, fmt.Errorf("compile embedded schema: %w", err)
	}
	return &Validator{ctx: ctx, schema: v}, nil
}

// ValidateReview validates raw JSON against #ReviewOutput and decodes it.
func (v *Validator) ValidateReview(data []byte) (*ReviewOutput, error) {
	var out ReviewOutput
	if err := v.Validate("#ReviewOutput", data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidateDescription validates raw JSON against #PRDescription and decodes it.
func (v *Validator) ValidateDescription(data []byte) (*PRDescription, error) {
	var out PRDescription
	if err := v.Validate("#PRDescription", data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Reconsideration mirrors #Reconsideration.
type Reconsideration struct {
	Verdict     string `json:"verdict"`
	Explanation string `json:"explanation"`
}

// ValidateReconsideration validates raw JSON against #Reconsideration and
// decodes it.
func (v *Validator) ValidateReconsideration(data []byte) (*Reconsideration, error) {
	var out Reconsideration
	if err := v.Validate("#Reconsideration", data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidateExplanation validates raw JSON against #Explanation and decodes it.
func (v *Validator) ValidateExplanation(data []byte) (*Explanation, error) {
	var out Explanation
	if err := v.Validate("#Explanation", data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidatePlan validates raw JSON against #ImplementationPlan and decodes it.
func (v *Validator) ValidatePlan(data []byte) (*ImplementationPlan, error) {
	var out ImplementationPlan
	if err := v.Validate("#ImplementationPlan", data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidateImprove validates raw JSON against #ImproveOutput and decodes it.
func (v *Validator) ValidateImprove(data []byte) (*ImproveOutput, error) {
	var out ImproveOutput
	if err := v.Validate("#ImproveOutput", data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Validate unifies raw JSON with the named definition (e.g. "#ReviewOutput"),
// requires the result to be concrete, and decodes it into out. Every failure
// caused by the data wraps ErrInvalid; an unknown definition does not.
func (v *Validator) Validate(definition string, data []byte, out any) error {
	if !json.Valid(data) {
		return fmt.Errorf("%w: response is not valid JSON", ErrInvalid)
	}
	expr, err := cuejson.Extract("response.json", data)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, details(err))
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	def := v.schema.LookupPath(cue.ParsePath(definition))
	if !def.Exists() {
		return fmt.Errorf("schema has no definition %q", definition)
	}
	u := def.Unify(v.ctx.BuildExpr(expr))
	if err := u.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, details(err))
	}
	if err := u.Decode(out); err != nil {
		return fmt.Errorf("%w: decode: %s", ErrInvalid, details(err))
	}
	return nil
}

// details flattens CUE's error list into one line per problem, which reads
// well both in logs and in a repair prompt sent back to the model.
func details(err error) string {
	errs := cueerrors.Errors(err)
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msg := e.Error()
		// "N errors in empty disjunction:" only announces the per-branch
		// conflicts that follow it, which are the useful part for a repair.
		if strings.HasSuffix(msg, "errors in empty disjunction:") {
			continue
		}
		if p := e.Path(); len(p) > 0 && !strings.HasPrefix(msg, strings.Join(p, ".")) {
			msg = strings.Join(p, ".") + ": " + msg
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return err.Error()
	}
	return strings.Join(msgs, "; ")
}
