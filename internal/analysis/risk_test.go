package analysis

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

func defaultPolicy(t *testing.T) *schema.RiskPolicy {
	t.Helper()
	v, err := schema.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.RiskPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// addPatch renders a diff adding n lines (built by line(i)) to path.
func addPatch(path string, n int, line func(i int) string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,1 +1,%d @@\n x\n", path, path, path, path, n+1)
	for i := range n {
		b.WriteString("+" + line(i) + "\n")
	}
	return b.String()
}

func parseDiff(t *testing.T, patch string) []*diff.File {
	t.Helper()
	d, err := diff.Parse(strings.NewReader(patch), diff.Options{Exclude: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return d.Files
}

func TestScoreRisk_DocsOnlyVsSecurityCritical(t *testing.T) {
	p := defaultPolicy(t)
	prose := func(i int) string { return fmt.Sprintf("Line %d of the guide.", i) }
	logic := func(i int) string { return fmt.Sprintf("\tif token == nil && attempt%d > 0 || expired {", i) }

	tests := []struct {
		name          string
		patch         string
		minScore      int
		maxScore      int
		belowApproval bool
	}{
		{"docs only", addPatch("docs/guide.md", 60, prose) + addPatch("README.md", 20, prose), 0, 10, true},
		{"tests only", addPatch("internal/auth/login_test.go", 30, logic), 0, 70, false},
		{"ordinary feature", addPatch("internal/api/list.go", 40, func(i int) string { return fmt.Sprintf("\tx%d := y", i) }), 15, 30, false},
		{"security critical", addPatch("internal/auth/login.go", 40, logic), 95, 100, false},
		{"schema change", addPatch("schema/review.cue", 20, func(i int) string { return fmt.Sprintf("f%d: int", i) }), 25, 35, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ScoreRisk(parseDiff(t, tt.patch), p)
			if r.Score < tt.minScore || r.Score > tt.maxScore {
				t.Errorf("score %d, want %d..%d (files %+v)", r.Score, tt.minScore, tt.maxScore, r.Files)
			}
			if below := r.Score <= p.AutoApproveThreshold; below != tt.belowApproval {
				t.Errorf("score %d vs threshold %d: below = %v, want %v", r.Score, p.AutoApproveThreshold, below, tt.belowApproval)
			}
		})
	}
}

func TestScoreRisk_Breakdown(t *testing.T) {
	p := defaultPolicy(t)
	patch := "diff --git a/internal/auth/login.go b/internal/auth/login.go\n--- a/internal/auth/login.go\n+++ b/internal/auth/login.go\n" +
		"@@ -1,3 +1,3 @@\n x\n-\tif a {\n+\tif a && b || c {\n x\n" +
		addPatch("docs/a.md", 4, func(int) string { return "text" })
	r := ScoreRisk(parseDiff(t, patch), p)
	if len(r.Files) != 2 || r.Files[0].Path != "internal/auth/login.go" {
		t.Fatalf("files not sorted by points: %+v", r.Files)
	}
	auth := r.Files[0]
	// 2 changed lines; decision points 3 added - 1 removed = 2; weight 3:
	// 3 × (2 + 5×2) = 36.
	if auth.Weight != 3 || auth.Changed != 2 || auth.ComplexityDelta != 2 || auth.Points != 36 {
		t.Errorf("auth breakdown %+v", auth)
	}
	if docs := r.Files[1]; docs.Weight != 0.1 || docs.Points != 0.4 {
		t.Errorf("docs breakdown %+v", docs)
	}
}

func TestScoreRisk_Rules(t *testing.T) {
	p := defaultPolicy(t)

	t.Run("first matching weight wins", func(t *testing.T) {
		if w := weightFor("internal/auth/login_test.go", p); w != 0.3 {
			t.Errorf("test file under auth weight %v, want 0.3 (tests listed first)", w)
		}
		if w := weightFor("internal/auth/login.go", p); w != 3 {
			t.Errorf("auth weight %v", w)
		}
		if w := weightFor("cmd/main.go", p); w != p.DefaultWeight {
			t.Errorf("default weight %v", w)
		}
	})

	t.Run("removing complexity never lowers the score below line churn", func(t *testing.T) {
		patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,1 @@\n-if a && b {\n-}\n x\n"
		r := ScoreRisk(parseDiff(t, patch), p)
		if f := r.Files[0]; f.ComplexityDelta != -2 || f.Points != 2 {
			t.Errorf("breakdown %+v, want delta -2 and points 2 (lines only)", f)
		}
	})

	t.Run("non-Go files have no complexity", func(t *testing.T) {
		r := ScoreRisk(parseDiff(t, addPatch("x.py", 1, func(int) string { return "if a and b or c:" })), p)
		if r.Files[0].ComplexityDelta != 0 {
			t.Errorf("python complexity %d", r.Files[0].ComplexityDelta)
		}
	})

	t.Run("binary files count", func(t *testing.T) {
		patch := "diff --git a/assets/x.png b/assets/x.png\nnew file mode 100644\nindex 0000000..1111111\nBinary files /dev/null and b/assets/x.png differ\n"
		if r := ScoreRisk(parseDiff(t, patch), p); r.Files[0].Points != 10 {
			t.Errorf("binary points %v", r.Files[0].Points)
		}
	})

	t.Run("bounded and monotonic", func(t *testing.T) {
		prev := -1
		for _, n := range []int{1, 10, 100, 1000, 100000} {
			r := ScoreRisk(parseDiff(t, addPatch("a.go", n, func(int) string { return "if x {" })), p)
			if r.Score < prev || r.Score > 100 {
				t.Errorf("n=%d score %d (prev %d)", n, r.Score, prev)
			}
			prev = r.Score
		}
		if prev != 100 {
			t.Errorf("huge diff score %d, want 100", prev)
		}
	})

	t.Run("empty diff scores zero", func(t *testing.T) {
		if r := ScoreRisk(nil, p); r.Score != 0 || len(r.Files) != 0 {
			t.Errorf("empty: %+v", r)
		}
	})
}

func TestRiskPolicy_SchemaDefaultsAndOverrides(t *testing.T) {
	v, err := schema.New()
	if err != nil {
		t.Fatal(err)
	}
	p := defaultPolicy(t)
	if p.DefaultWeight != 1 || p.Scale != 150 || p.AutoApproveThreshold != 20 || p.SensitiveWeight != 2.5 || len(p.Weights) != 9 {
		t.Errorf("defaults %+v", p)
	}

	o, err := v.RiskPolicy([]byte(`{"auto_approve_threshold": 5, "weights": [{"pattern": "**/billing/**", "weight": 4}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if o.AutoApproveThreshold != 5 || len(o.Weights) != 1 || o.Scale != 150 {
		t.Errorf("override %+v", o)
	}

	for name, bad := range map[string]string{
		"weight too high": `{"weights":[{"pattern":"a","weight":11}]}`,
		"empty pattern":   `{"weights":[{"pattern":"","weight":1}]}`,
		"zero scale":      `{"scale":0}`,
		"threshold > 100": `{"auto_approve_threshold":101}`,
		"unknown field":   `{"approve_all":true}`,
	} {
		if _, err := v.RiskPolicy([]byte(bad)); !errors.Is(err, schema.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}

	if err := ValidateRiskPatterns(&schema.RiskPolicy{Weights: []schema.RiskWeight{{Pattern: "[", Weight: 1}}}); err == nil {
		t.Error("malformed glob accepted")
	}
}
