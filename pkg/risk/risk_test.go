package risk_test

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
	"github.com/infrashift/vizzini-pr-reviewer/pkg/risk"
)

// authPatch changes one auth line (decision points 1 → 3) and adds four
// lines of docs: 36 and 0.4 points under the default policy.
const authPatch = "diff --git a/internal/auth/login.go b/internal/auth/login.go\n--- a/internal/auth/login.go\n+++ b/internal/auth/login.go\n" +
	"@@ -1,3 +1,3 @@\n x\n-\tif a {\n+\tif a && b || c {\n x\n" +
	"diff --git a/docs/a.md b/docs/a.md\n--- a/docs/a.md\n+++ b/docs/a.md\n@@ -1,1 +1,5 @@\n x\n+t\n+t\n+t\n+t\n" +
	"diff --git a/go.sum b/go.sum\n--- a/go.sum\n+++ b/go.sum\n@@ -1,1 +1,2 @@\n x\n+y\n"

func TestDefaultPolicy(t *testing.T) {
	p := risk.DefaultPolicy()
	if p.DefaultWeight != 1 || p.Scale != 150 || p.AutoApproveThreshold != 20 || p.SensitiveWeight != 2.5 || p.HighThreshold != 60 || len(p.Weights) != 9 {
		t.Errorf("defaults %+v", p)
	}
	// Each call is a fresh copy, so a caller tweaking one cannot leak into
	// another.
	p.Scale = 1
	if risk.DefaultPolicy().Scale != 150 {
		t.Error("DefaultPolicy shares state between calls")
	}
}

func TestLoadPolicy(t *testing.T) {
	p, err := risk.LoadPolicy([]byte(`{"auto_approve_threshold": 70, "high_threshold": 80, "weights": [{"pattern": "**/billing/**", "weight": 4}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.AutoApproveThreshold != 70 || p.HighThreshold != 80 || len(p.Weights) != 1 || p.Scale != 150 {
		t.Errorf("override %+v", p)
	}

	for name, bad := range map[string]string{
		"high not above approve": `{"auto_approve_threshold": 30, "high_threshold": 30}`,
		// Raising the approve threshold past the default high threshold
		// without moving the high one is a mistake, not a silent reorder.
		"default high below raised approve": `{"auto_approve_threshold": 70}`,
		"high above 100":                    `{"high_threshold": 101}`,
		"unknown field":                     `{"approve_all": true}`,
		"not JSON":                          `{`,
	} {
		if _, err := risk.LoadPolicy([]byte(bad)); !errors.Is(err, schema.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := risk.LoadPolicy([]byte(`{"weights": [{"pattern": "[", "weight": 1}]}`)); err == nil {
		t.Error("malformed glob accepted")
	}
}

func TestPolicyBandAndSensitivity(t *testing.T) {
	p := risk.DefaultPolicy() // low ≤ 20 < medium < 60 ≤ high
	for score, want := range map[int]risk.Band{
		0: risk.BandLow, 20: risk.BandLow, 21: risk.BandMedium, 59: risk.BandMedium, 60: risk.BandHigh, 100: risk.BandHigh,
	} {
		if got := p.Band(score); got != want {
			t.Errorf("Band(%d) = %s, want %s", score, got, want)
		}
	}
	if !p.IsSensitive(2.5) || !p.IsSensitive(3) || p.IsSensitive(2.49) {
		t.Error("IsSensitive must be weight >= sensitive_weight")
	}
}

func TestAssessPatch(t *testing.T) {
	p := risk.DefaultPolicy()
	a, err := risk.AssessPatch(strings.NewReader(authPatch), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Files) != 2 {
		t.Fatalf("go.sum is excluded by default; files %+v", a.Files)
	}
	auth, docs := a.Files[0], a.Files[1]
	if auth.Path != "internal/auth/login.go" || auth.Weight != 3 || auth.Changed != 2 || auth.ComplexityDelta != 2 || auth.Points != 36 {
		t.Errorf("auth %+v", auth)
	}
	if docs.Path != "docs/a.md" || docs.Points != 0.4 {
		t.Errorf("docs %+v", docs)
	}
	if want := risk.Combine(a.Files, p); a.Score != want || a.Score != 22 || a.Band != risk.BandMedium {
		t.Errorf("score %d band %s, want 22 medium (Combine %d)", a.Score, a.Band, want)
	}

	all, err := risk.AssessPatch(strings.NewReader(authPatch), p, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Files) != 3 {
		t.Errorf("an empty exclude list keeps go.sum; files %+v", all.Files)
	}

	if _, err := risk.AssessPatch(strings.NewReader(authPatch), p, []string{"["}); err == nil {
		t.Error("malformed exclude pattern accepted")
	}
}

// An embedder with its own diff model must get the same numbers without
// rendering a patch, and per-entry results it can align with its own list.
func TestScore_MatchesAssessPatchAndKeepsDuplicates(t *testing.T) {
	p := risk.DefaultPolicy()
	changes := []risk.FileChange{
		{Path: "internal/auth/login.go", Added: []string{"\tif a && b || c {"}, Deleted: []string{"\tif a {"}},
		{Path: "docs/a.md", Added: []string{"t", "t", "t", "t"}},
	}
	got := risk.Score(changes, p)
	want, err := risk.AssessPatch(strings.NewReader(authPatch), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
		t.Errorf("Score %+v\nAssessPatch %+v", got, want)
	}

	// A patch series touching one path twice: two entries, scored apart,
	// in the caller's order.
	series := []risk.FileChange{
		{Path: "a.go", Added: []string{"x"}},
		{Path: "a.go", Added: []string{"if a {", "y"}},
		{Path: "assets/logo.png", Binary: true},
	}
	per := make([]risk.FileRisk, 0, len(series))
	for i := range series {
		per = append(per, risk.ScoreFile(&series[i], p))
	}
	if per[0].Points != 1 || per[1].Points != 7 || per[2].Points != 10 {
		t.Errorf("per-entry points %+v", per)
	}
	if risk.Combine(per, p) != risk.Score(series, p).Score {
		t.Error("Combine over ScoreFile disagrees with Score")
	}
}

// An embedder filtering its own files with an Excluder must drop exactly
// what AssessPatch drops, or its score drifts from the review's.
func TestExcluder_MatchesAssessPatch(t *testing.T) {
	ex, err := risk.NewExcluder(nil)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"go.sum": true, "sub/go.sum": true, "vendor/a/b.go": true, "api/x.pb.go": true, "Cargo.lock": true,
		"internal/auth/login.go": false, "docs/a.md": false, "vendored/a.go": false,
	} {
		if got := ex.Excluded(path); got != want {
			t.Errorf("Excluded(%q) = %v, want %v", path, got, want)
		}
	}

	p := risk.DefaultPolicy()
	whole, err := risk.AssessPatch(strings.NewReader(authPatch), p, []string{})
	if err != nil {
		t.Fatal(err)
	}
	var kept []risk.FileRisk
	for _, f := range whole.Files {
		if !ex.Excluded(f.Path) {
			kept = append(kept, f)
		}
	}
	filtered, err := risk.AssessPatch(strings.NewReader(authPatch), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if risk.Combine(kept, p) != filtered.Score || len(kept) != len(filtered.Files) {
		t.Errorf("Excluder keeps %+v, AssessPatch %+v", kept, filtered.Files)
	}

	none, err := risk.NewExcluder([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if none.Excluded("go.sum") {
		t.Error("an empty exclude list must keep everything")
	}
	if _, err := risk.NewExcluder([]string{"["}); err == nil {
		t.Error("malformed exclude pattern accepted")
	}

	d := risk.DefaultExcludes()
	d[0] = "changed"
	if risk.DefaultExcludes()[0] == "changed" {
		t.Error("DefaultExcludes shares its backing array")
	}
}

// The package's reason to exist is that embedders can link it without the
// LLM stack. Guard that against a future import.
func TestPackageIsADependencyLeaf(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	out, err := exec.CommandContext(t.Context(), gobin, "list", "-deps", ".").Output() //nolint:gosec // G204: fixed arguments
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	deps := string(out)
	for _, banned := range []string{
		"github.com/sashabaranov/go-openai",
		"github.com/pkoukk/tiktoken-go",
		"github.com/infrashift/vizzini-pr-reviewer/internal/llm",
		"github.com/infrashift/vizzini-pr-reviewer/internal/chunker",
		"github.com/infrashift/vizzini-pr-reviewer/internal/okf",
		"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer",
	} {
		for _, d := range strings.Split(deps, "\n") {
			if d == banned || strings.HasPrefix(d, banned+"/") {
				t.Errorf("pkg/risk depends on %s", d)
			}
		}
	}
}
