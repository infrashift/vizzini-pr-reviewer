package reviewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
	"github.com/infrashift/vizzini-pr-reviewer/pkg/risk"
)

// APIVersion identifies Vizzini's JSON reports (ADR-0010). Policy engines
// such as garmr target reports by its group, "vizzini.infrashift.io", and
// by kind.
const APIVersion = risk.APIVersion

// Report kinds, one per result type.
const (
	KindReview             = "Review"
	KindPRDescription      = "PRDescription"
	KindSuggestions        = "Suggestions"
	KindCommandResult      = "CommandResult"
	KindImplementationPlan = "ImplementationPlan"
	// KindRiskAssessment is the deterministic score alone, from
	// pr-review --mode risk (ISSUE-015).
	KindRiskAssessment = "RiskAssessment"
)

var reportValidator = sync.OnceValues(schema.New)

// MarshalReport encodes a result as an indented JSON report: the result's
// own fields preceded by "apiVersion" and "kind". Review reports are
// validated against the closed #ReviewReport schema and every other kind
// against the #Report envelope, so consumers can rely on the shape.
// Improve suggestions are wrapped as {"suggestions": [...]}. A
// *risk.Assessment becomes a RiskAssessment report, checked against the
// closed #RiskAssessmentReport.
func MarshalReport(v any) ([]byte, error) {
	return marshalReport(v, nil)
}

// MarshalReportWithRun is MarshalReport plus run's ID, attribution and
// usage, for report kinds whose result has no room for them (Describe,
// Improve, Plan, RunCommand; collect run with a RunRecorder). A nil run
// adds nothing. A *ReviewOutput already carries its own, so run is ignored.
func MarshalReportWithRun(v any, run *RunInfo) ([]byte, error) {
	if _, isReview := v.(*ReviewOutput); isReview || run == nil {
		return marshalReport(v, nil)
	}
	extra, err := json.Marshal(struct {
		Attribution
		Usage *Usage `json:"usage,omitempty"`
	}{run.Attribution, &run.Usage})
	if err != nil {
		return nil, err
	}
	return marshalReport(v, extra)
}

// marshalReport encodes v as a report; extra, a JSON object, adds fields.
func marshalReport(v any, extra []byte) ([]byte, error) {
	var (
		kind string
		body = v
	)
	switch x := v.(type) {
	case *ReviewOutput:
		kind = KindReview
	case *PRDescription:
		kind = KindPRDescription
	case []CodeSuggestion:
		kind = KindSuggestions
		if x == nil {
			x = []CodeSuggestion{}
		}
		body = struct {
			Suggestions []CodeSuggestion `json:"suggestions"`
		}{x}
	case *CommandResult:
		kind = KindCommandResult
	case *ImplementationPlan:
		kind = KindImplementationPlan
	case *risk.Assessment:
		kind = KindRiskAssessment
		if x.Files == nil {
			a := *x
			a.Files = []risk.FileRisk{}
			body = &a
		}
	default:
		return nil, fmt.Errorf("reviewer: no report kind for %T", v)
	}

	fields, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if len(fields) < 2 || fields[0] != '{' {
		return nil, fmt.Errorf("reviewer: %T does not encode as a JSON object", v)
	}
	var raw bytes.Buffer
	fmt.Fprintf(&raw, `{"apiVersion":%q,"kind":%q`, APIVersion, kind)
	if len(fields) > 2 { // not "{}"
		raw.WriteByte(',')
	}
	if len(extra) > 2 { // not "{}"
		raw.Write(fields[1 : len(fields)-1])
		if len(fields) > 2 {
			raw.WriteByte(',')
		}
		raw.Write(extra[1:])
	} else {
		raw.Write(fields[1:])
	}

	val, err := reportValidator()
	if err != nil {
		return nil, err
	}
	def := "#Report"
	switch kind {
	case KindReview:
		def = "#ReviewReport"
	case KindRiskAssessment:
		def = "#RiskAssessmentReport"
	}
	var discard map[string]any
	if err := val.Validate(def, raw.Bytes(), &discard); err != nil {
		return nil, fmt.Errorf("reviewer: %s report: %w", kind, err)
	}

	var out bytes.Buffer
	if err := json.Indent(&out, raw.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
