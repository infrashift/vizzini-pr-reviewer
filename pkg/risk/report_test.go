package risk_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/risk"
)

const envelope = `{
  "apiVersion": "vizzini.infrashift.io/v1",
  "kind": "Review",
  "summary": "s",
  "risk_score": 72,
  "base_risk_score": 41,
  "model_risk_score": 72,
  "risk_files": [{"path": "internal/auth/login.go", "weight": 3, "changed": 2, "complexity_delta": 2, "points": 36}],
  "auto_approve": false,
  "findings": [
    {"file": "internal/store/users.go", "start_line": 44, "end_line": 45, "severity": "Critical", "category": "Security",
     "title": "SQL built from input", "body": "Use a parameter.", "suggestion": "db.Query(q, name)", "citations": ["adr-0001"],
     "a_finding_field_from_a_newer_vizzini": 1},
    {"file": "internal/auth/login.go", "start_line": 3, "end_line": 3, "severity": "Minor", "category": "Logic",
     "title": "t", "body": "b"}
  ],
  "knowledge_docs": ["adr-0001"],
  "chunks": 1,
  "a_field_from_a_newer_vizzini": true
}`

const bare = `{"summary":"s","risk_score":12,"base_risk_score":12,"model_risk_score":5,"auto_approve":true,"findings":[],"chunks":1}`

func TestParseReport(t *testing.T) {
	r, err := risk.ParseReport([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if r.RiskScore != 72 || r.BaseRiskScore != 41 || r.ModelRiskScore != 72 || r.AutoApprove || len(r.RiskFiles) != 1 || r.RiskFiles[0].Points != 36 {
		t.Errorf("envelope %+v", r)
	}
	if r.Divergence() != 31 {
		t.Errorf("divergence %d, want 31", r.Divergence())
	}
	if r.Summary != "s" || len(r.KnowledgeDocs) != 1 || r.KnowledgeDocs[0] != "adr-0001" {
		t.Errorf("summary %q, knowledge %v", r.Summary, r.KnowledgeDocs)
	}
	want := risk.Finding{
		File: "internal/store/users.go", StartLine: 44, EndLine: 45, Severity: "Critical", Category: "Security",
		Title: "SQL built from input", Body: "Use a parameter.", Suggestion: "db.Query(q, name)", Citations: []string{"adr-0001"},
	}
	if len(r.Findings) != 2 || !reflect.DeepEqual(r.Findings[0], want) || r.Findings[1].Suggestion != "" || r.Findings[1].Citations != nil {
		t.Errorf("findings %+v", r.Findings)
	}

	b, err := risk.ParseReport([]byte(bare))
	if err != nil {
		t.Fatal(err)
	}
	if b.RiskScore != 12 || !b.AutoApprove || b.RiskFiles != nil || b.Findings == nil || len(b.Findings) != 0 {
		t.Errorf("bare %+v", b)
	}
	if b.Divergence() != 0 {
		t.Errorf("model below base must not diverge: %d", b.Divergence())
	}
}

func TestParseReport_Rejects(t *testing.T) {
	for name, data := range map[string]string{
		"not JSON":              `{`,
		"trailing data":         bare + `{}`,
		"other kind":            `{"apiVersion":"vizzini.infrashift.io/v1","kind":"RiskAssessment","score":5,"band":"low","files":[]}`,
		"other version":         `{"apiVersion":"vizzini.infrashift.io/v2","kind":"Review","risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[]}`,
		"apiVersion only":       `{"apiVersion":"vizzini.infrashift.io/v1","risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[]}`,
		"kind only":             `{"kind":"Review","risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[]}`,
		"missing base":          `{"risk_score":1,"model_risk_score":1,"auto_approve":false,"findings":[]}`,
		"missing findings":      `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false}`,
		"null findings":         `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":null}`,
		"missing auto_approve":  `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"findings":[]}`,
		"out of range":          `{"risk_score":150,"base_risk_score":150,"model_risk_score":0,"auto_approve":false,"findings":[]}`,
		"negative":              `{"risk_score":0,"base_risk_score":0,"model_risk_score":-1,"auto_approve":false,"findings":[]}`,
		"risk not max":          `{"risk_score":10,"base_risk_score":40,"model_risk_score":10,"auto_approve":false,"findings":[]}`,
		"file without path":     `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[],"risk_files":[{"path":"","weight":1,"changed":1,"complexity_delta":0,"points":1}]}`,
		"an array":              `[]`,
		"finding not an object": `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[1]}`,
		"finding without file":  `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[{"file":"","start_line":1,"end_line":1,"severity":"Minor","category":"Logic","title":"t","body":"b"}]}`,
		"finding line zero":     `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[{"file":"a.go","start_line":0,"end_line":1,"severity":"Minor","category":"Logic","title":"t","body":"b"}]}`,
		"finding ends early":    `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[{"file":"a.go","start_line":5,"end_line":4,"severity":"Minor","category":"Logic","title":"t","body":"b"}]}`,
		"finding no severity":   `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[{"file":"a.go","start_line":1,"end_line":1,"category":"Logic","title":"t","body":"b"}]}`,
		"finding no title":      `{"risk_score":1,"base_risk_score":1,"model_risk_score":0,"auto_approve":false,"findings":[{"file":"a.go","start_line":1,"end_line":1,"severity":"Minor","category":"Logic","body":"b"}]}`,
	} {
		if _, err := risk.ParseReport([]byte(data)); !errors.Is(err, risk.ErrNotReviewReport) {
			t.Errorf("%s: err = %v, want ErrNotReviewReport", name, err)
		}
	}
}
