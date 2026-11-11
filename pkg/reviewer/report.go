package reviewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// APIVersion identifies Vizzini's JSON reports (ADR-0010). Policy engines
// such as garmr target reports by its group, "vizzini.infrashift.io", and
// by kind.
const APIVersion = "vizzini.infrashift.io/v1"

// Report kinds, one per result type.
const (
	KindReview             = "Review"
	KindPRDescription      = "PRDescription"
	KindSuggestions        = "Suggestions"
	KindCommandResult      = "CommandResult"
	KindImplementationPlan = "ImplementationPlan"
)

var reportValidator = sync.OnceValues(schema.New)

// MarshalReport encodes a result as an indented JSON report: the result's
// own fields preceded by "apiVersion" and "kind". Review reports are
// validated against the closed #ReviewReport schema and every other kind
// against the #Report envelope, so consumers can rely on the shape.
// Improve suggestions are wrapped as {"suggestions": [...]}.
func MarshalReport(v any) ([]byte, error) {
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
	raw.Write(fields[1:])

	val, err := reportValidator()
	if err != nil {
		return nil, err
	}
	def := "#Report"
	if kind == KindReview {
		def = "#ReviewReport"
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
