package reviewer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

func decodeReport(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, b)
	}
	return m
}

func TestMarshalReport_Review(t *testing.T) {
	r := &ReviewOutput{
		Summary: "s", RiskScore: 40, BaseRiskScore: 40, ModelRiskScore: 10, Chunks: 1,
		Findings:  []Finding{{File: "a.go", StartLine: 1, EndLine: 2, Severity: SeverityMajor, Category: CategoryLogic, Title: "t", Body: "b"}},
		RiskFiles: []FileRisk{{Path: "a.go", Weight: 1, Changed: 2, ComplexityDelta: 1, Points: 7}},
	}
	b, err := MarshalReport(r)
	if err != nil {
		t.Fatal(err)
	}
	// Envelope first, then the result's own fields in struct order.
	if !strings.HasPrefix(string(b), "{\n  \"apiVersion\": \"vizzini.infrashift.io/v1\",\n  \"kind\": \"Review\",\n  \"summary\": \"s\",") {
		t.Errorf("report does not lead with the envelope:\n%s", b)
	}
	m := decodeReport(t, b)
	findings, _ := m["findings"].([]any)
	if m["risk_score"] != 40.0 || m["auto_approve"] != false || len(findings) != 1 {
		t.Errorf("fields lost: %v", m)
	}
	if !strings.HasSuffix(string(b), "}\n") {
		t.Error("report should end with a newline")
	}
}

// The report schema is closed and constrained, so a corrupt result cannot be
// printed as if it were valid.
func TestMarshalReport_ReviewIsValidated(t *testing.T) {
	for name, r := range map[string]*ReviewOutput{
		"risk above 100":   {Summary: "s", RiskScore: 150, Findings: []Finding{}},
		"invalid severity": {Summary: "s", Findings: []Finding{{File: "a.go", StartLine: 1, EndLine: 1, Severity: "High", Category: CategoryLogic, Title: "t", Body: "b"}}},
		"nil findings":     {Summary: "s"},
	} {
		if _, err := MarshalReport(r); err == nil || !strings.Contains(err.Error(), "Review report") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestMarshalReport_OtherKinds(t *testing.T) {
	tests := []struct {
		v    any
		kind string
		key  string
	}{
		{&PRDescription{Title: "t", Type: PRTypeDocs, Summary: "s", Changes: []FileChange{}}, KindPRDescription, "title"},
		{[]CodeSuggestion{{File: "a.go", StartLine: 1, EndLine: 1, Category: CategoryLogic, Summary: "s", ImprovedCode: "x"}}, KindSuggestions, "suggestions"},
		{&CommandResult{Command: Command{Kind: CommandExplain, File: "a.go", StartLine: 1, EndLine: 1}, Answer: "a"}, KindCommandResult, "answer"},
		{&ImplementationPlan{Summary: "s", Verification: []string{"make test"}}, KindImplementationPlan, "verification"},
	}
	for _, tt := range tests {
		b, err := MarshalReport(tt.v)
		if err != nil {
			t.Fatalf("%s: %v", tt.kind, err)
		}
		m := decodeReport(t, b)
		if m["apiVersion"] != APIVersion || m["kind"] != tt.kind || m[tt.key] == nil {
			t.Errorf("%s report: %v", tt.kind, m)
		}
	}

	// nil suggestions still produce a well-formed list.
	b, err := MarshalReport([]CodeSuggestion(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"suggestions": []`) {
		t.Errorf("nil suggestions:\n%s", b)
	}

	if _, err := MarshalReport(42); err == nil {
		t.Error("unsupported type accepted")
	}
}

// The Go constant and the CUE #APIVersion must agree.
func TestAPIVersionMatchesSchema(t *testing.T) {
	v, err := schema.New()
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := v.Validate("#Report", []byte(`{"apiVersion":"`+APIVersion+`","kind":"Review"}`), &out); err != nil {
		t.Errorf("APIVersion %q rejected by #Report: %v", APIVersion, err)
	}
}

func TestMarshalReportWithRun_ValidatesUsage(t *testing.T) {
	bad := &RunInfo{Attribution: Attribution{RunID: "r"}, Usage: Usage{Requests: -1}}
	if _, err := MarshalReportWithRun(&PRDescription{Title: "t", Type: "Feature", Summary: "s", Changes: []FileChange{}}, bad); err == nil {
		t.Error("a negative request count must fail the #Report schema")
	}
}
