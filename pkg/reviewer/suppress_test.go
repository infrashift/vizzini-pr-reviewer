package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

func TestParseSuppressions_YAMLAndJSON(t *testing.T) {
	yaml := `
suppressions:
  - id: SUP-1
    reason: The legacy formatter is frozen until its rewrite.
    paths: ["src/Legacy/**"]
    categories: [Maintainability]
  - id: SUP-2
    reason: Accepted risk, see ADR-0007.
    title: hardcoded credential
    expires: "2027-01-31"
`
	s, err := ParseSuppressions([]byte(yaml))
	if err != nil || s.Len() != 2 {
		t.Fatalf("yaml: %v (%d rules)", err, s.Len())
	}
	j, err := ParseSuppressions([]byte(`{"suppressions":[{"id":"S","reason":"r","fingerprint":"v1:0123456789abcdef"}]}`))
	if err != nil || j.Len() != 1 {
		t.Fatalf("json: %v", err)
	}
	for name, bad := range map[string]string{
		"not yaml":        "suppressions: [",
		"nothing named":   "suppressions:\n  - id: S\n    reason: r\n    categories: [Logic]\n",
		"unknown field":   "suppressions:\n  - id: S\n    reason: r\n    title: x\n    severity: Minor\n",
		"top-level typo":  "suppression:\n  - id: S\n    reason: r\n    title: x\n",
		"duplicate rules": "suppressions:\n  - {id: S, reason: r, title: x}\n  - {id: S, reason: r, title: y}\n",
	} {
		if _, err := ParseSuppressions([]byte(bad)); err == nil {
			t.Errorf("%s: parsed", name)
		} else if name != "not yaml" && !errors.Is(err, schema.ErrInvalid) {
			t.Errorf("%s: %v, want a schema error", name, err)
		}
	}
	var none *Suppressions
	if none.Len() != 0 || none.Match(&Finding{}, time.Now()) != nil {
		t.Error("nil rules must match nothing")
	}
}

func TestSuppressions_Match(t *testing.T) {
	f := Finding{File: "src/Legacy/ReportFormatter.cs", Category: CategoryMaintainability, Title: "Unused variable width",
		Severity: SeverityMinor}
	now := time.Date(2027, 1, 31, 23, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		rule  string
		match bool
	}{
		{`{id: A, reason: r, paths: ["src/Legacy/**"]}`, true},
		{`{id: A, reason: r, paths: ["*.cs"]}`, true}, // a slash-free glob matches the base name
		{`{id: A, reason: r, paths: ["src/Services/**", "src/Legacy/*.cs"]}`, true},
		{`{id: A, reason: r, paths: ["src/Services/**"]}`, false},
		{`{id: A, reason: r, paths: ["src/Legacy/**"], categories: [Security]}`, false},
		{`{id: A, reason: r, paths: ["src/Legacy/**"], categories: [Logic, Maintainability]}`, true},
		{`{id: A, reason: r, title: "UNUSED variable"}`, true},
		{`{id: A, reason: r, title: "unused import"}`, false},
		{`{id: A, reason: r, fingerprint: "` + Fingerprint(&f) + `"}`, true},
		{`{id: A, reason: r, fingerprint: "v1:0000000000000000"}`, false},
		{`{id: A, reason: r, title: unused, expires: "2027-01-31"}`, true}, // valid through its last day
		{`{id: A, reason: r, title: unused, expires: "2027-01-30"}`, false},
	} {
		s, err := ParseSuppressions([]byte("suppressions:\n  - " + tc.rule + "\n"))
		if err != nil {
			t.Fatalf("%s: %v", tc.rule, err)
		}
		if got := s.Match(&f, now) != nil; got != tc.match {
			t.Errorf("%s: match %v, want %v", tc.rule, got, tc.match)
		}
	}
}

// End to end: a suppressed finding leaves Findings for Suppressed, the gate
// stops seeing it, auto-approval still does, and the report validates.
func TestReview_Suppressions(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	rules, err := ParseSuppressions([]byte(`
suppressions:
  - id: SQL-ACCEPTED
    reason: Users are looked up by an internal admin tool only.
    paths: ["internal/store/**"]
    categories: [Security]
`))
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine(t, srv)
	out, err := e.Review(context.Background(), fixture(t), ReviewOptions{Suppressions: rules})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Findings) != 1 || out.Findings[0].File != "internal/api/handler.go" {
		t.Errorf("findings %+v, want only the handler's", out.Findings)
	}
	if len(out.Suppressed) != 1 || out.Suppressed[0].Rule != "SQL-ACCEPTED" ||
		out.Suppressed[0].Finding.Severity != SeverityCritical || !strings.Contains(out.Suppressed[0].Reason, "admin tool") {
		t.Errorf("suppressed %+v", out.Suppressed)
	}
	if out.AutoApprove {
		t.Error("a suppressed Critical must still rule out auto-approval")
	}
	gate, err := e.Policy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if blocking := gate.Blocking(out.Findings); len(blocking) != 0 {
		t.Errorf("the gate must not see suppressed findings: %+v", blocking)
	}

	report, err := MarshalReport(out)
	if err != nil {
		t.Fatalf("the report with suppressed findings must validate: %v", err)
	}
	var back struct {
		Suppressed []SuppressedFinding `json:"suppressed"`
	}
	if uerr := json.Unmarshal(report, &back); uerr != nil || len(back.Suppressed) != 1 {
		t.Errorf("report %s (%v)", report, uerr)
	}

	// Without rules nothing is suppressed and the field is absent.
	out, err = e.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil || len(out.Findings) != 2 || out.Suppressed != nil {
		t.Errorf("no rules: %+v %v", out, err)
	}
}

func TestFingerprint(t *testing.T) {
	f := Finding{File: "a.go", StartLine: 3, EndLine: 3, Category: "Security", Title: "SQL  Injection"}
	moved := f
	moved.StartLine, moved.EndLine, moved.Title = 40, 41, "sql injection"
	if Fingerprint(&f) != Fingerprint(&moved) {
		t.Error("a moved or re-cased finding changed fingerprint")
	}
	for _, change := range []func(*Finding){
		func(g *Finding) { g.File = "b.go" },
		func(g *Finding) { g.Category = "Logic" },
		func(g *Finding) { g.Title = "Hardcoded secret" },
	} {
		g := f
		change(&g)
		if Fingerprint(&g) == Fingerprint(&f) {
			t.Errorf("%+v shares a fingerprint with %+v", g, f)
		}
	}
	// The recipe is versioned and stable: thread markers already carry it.
	if fp := Fingerprint(&f); fp != "v1:"+fp[3:] || len(fp) != 19 {
		t.Errorf("fingerprint %q", fp)
	}
}
