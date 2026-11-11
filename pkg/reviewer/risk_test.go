package reviewer

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/pkg/risk"
)

// ISSUE-015: what an embedder computes with pkg/risk must be exactly what a
// review reports, for the same diff and policy.
func TestReview_BaseRiskMatchesPkgRisk(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	for name, policy := range map[string][]byte{
		"default": nil,
		"custom":  []byte(`{"scale": 20, "weights": [{"pattern": "**/store/**", "weight": 4}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := newEngine(t, srv, func(c *Config) { c.RiskPolicy = policy }).Review(context.Background(), fixture(t), ReviewOptions{})
			if err != nil {
				t.Fatal(err)
			}
			p, err := risk.LoadPolicy(policy)
			if err != nil {
				t.Fatal(err)
			}
			a, err := risk.AssessPatch(fixture(t), p, nil)
			if err != nil {
				t.Fatal(err)
			}
			if a.Score != out.BaseRiskScore || !reflect.DeepEqual(a.Files, out.RiskFiles) {
				t.Errorf("pkg/risk %d %+v\nReview   %d %+v", a.Score, a.Files, out.BaseRiskScore, out.RiskFiles)
			}
			if name == "custom" && a.Score == 0 {
				t.Error("custom policy should give the fixture a non-zero score")
			}
		})
	}
}

// A report pr-review prints must read back through risk.ParseReport.
func TestMarshalReport_ReviewParsesAsRiskReport(t *testing.T) {
	out, err := newEngine(t, llmtest.NewServer(t, answerByFile)).Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalReport(out)
	if err != nil {
		t.Fatal(err)
	}
	r, err := risk.ParseReport(b)
	if err != nil {
		t.Fatal(err)
	}
	if r.RiskScore != out.RiskScore || r.BaseRiskScore != out.BaseRiskScore || r.ModelRiskScore != out.ModelRiskScore ||
		r.AutoApprove != out.AutoApprove || !reflect.DeepEqual(r.RiskFiles, out.RiskFiles) || r.Summary != out.Summary {
		t.Errorf("round trip lost fields:\nreport %+v\nreview %+v", r, out)
	}
	if len(out.Findings) == 0 {
		t.Fatal("the fixture review should have findings")
	}
	if len(r.Findings) != len(out.Findings) {
		t.Fatalf("%d findings read back, want %d", len(r.Findings), len(out.Findings))
	}
	for i, f := range out.Findings {
		want := risk.Finding{
			File: f.File, StartLine: f.StartLine, EndLine: f.EndLine, Severity: string(f.Severity), Category: string(f.Category),
			Title: f.Title, Body: f.Body, Suggestion: f.Suggestion, Citations: f.Citations,
		}
		if !reflect.DeepEqual(r.Findings[i], want) {
			t.Errorf("finding %d:\nrisk     %+v\nreviewer %+v", i, r.Findings[i], f)
		}
	}
}

// ISSUE-017: risk.Finding is a CUE-free copy of reviewer.Finding for
// embedders. The two must keep the same fields, types and JSON names, or a
// field added to one silently drops out of the other.
func TestRiskFindingMirrorsFinding(t *testing.T) {
	shape := func(typ reflect.Type) []string {
		out := make([]string, 0, typ.NumField())
		for i := range typ.NumField() {
			f := typ.Field(i)
			out = append(out, f.Name+" "+f.Type.Kind().String()+" "+f.Tag.Get("json"))
		}
		return out
	}
	if a, b := shape(reflect.TypeFor[Finding]()), shape(reflect.TypeFor[risk.Finding]()); !reflect.DeepEqual(a, b) {
		t.Errorf("shapes differ:\nreviewer.Finding %q\nrisk.Finding     %q", a, b)
	}
}

func TestMarshalReport_RiskAssessment(t *testing.T) {
	a := &risk.Assessment{Score: 22, Band: risk.BandMedium, Files: []risk.FileRisk{{Path: "a.go", Weight: 1, Changed: 2, Points: 2}}}
	b, err := MarshalReport(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "{\n  \"apiVersion\": \"vizzini.infrashift.io/v1\",\n  \"kind\": \"RiskAssessment\",\n  \"score\": 22,\n  \"band\": \"medium\",") {
		t.Errorf("report:\n%s", b)
	}

	empty, err := MarshalReport(&risk.Assessment{Band: risk.BandLow})
	if err != nil {
		t.Fatal(err)
	}
	if m := decodeReport(t, empty); m["files"] == nil {
		t.Errorf("nil files must encode as [], got %v", m)
	}

	for name, bad := range map[string]*risk.Assessment{
		"no band":      {Score: 5, Files: []risk.FileRisk{}},
		"score > 100":  {Score: 101, Band: risk.BandHigh},
		"unknown band": {Score: 5, Band: "severe"},
	} {
		if _, err := MarshalReport(bad); err == nil || !strings.Contains(err.Error(), "RiskAssessment report") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestFormatRiskAssessment(t *testing.T) {
	a := &risk.Assessment{Score: 22, Band: risk.BandMedium, Files: []risk.FileRisk{
		{Path: "internal/auth/login.go", Weight: 3, Changed: 2, ComplexityDelta: 2, Points: 36},
		{Path: "docs/a.md", Weight: 0.1, Changed: 4, Points: 0.4},
	}}
	want := "## Risk\n\n" +
		"**Risk:** 22/100 (medium) · computed from the diff alone, no model\n\n" +
		"- `internal/auth/login.go`: 36 points (weight 3, 2 changed, complexity +2)\n" +
		"- `docs/a.md`: 0.4 points (weight 0.1, 4 changed, complexity +0)\n"
	if got := FormatRiskAssessment(a); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := FormatRiskAssessment(&risk.Assessment{Band: risk.BandLow}); got != "## Risk\n\n**Risk:** 0/100 (low) · computed from the diff alone, no model\n" {
		t.Errorf("empty: %q", got)
	}
}

func TestAPIVersionIsShared(t *testing.T) {
	if APIVersion != risk.APIVersion {
		t.Errorf("reviewer %q, risk %q", APIVersion, risk.APIVersion)
	}
}
