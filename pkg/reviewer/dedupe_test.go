package reviewer

import (
	"reflect"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

const (
	adrExec    = "01a1cb22-3889-7604-b838-d3abb064d485"
	adrEnvOnly = "01a1cb22-389c-7d56-bb22-00d672f96dec"
)

func f(file string, start, end int, sev schema.Severity, cat schema.Category, title string, cites ...string) Finding {
	return Finding{File: file, StartLine: start, EndLine: end, Severity: sev, Category: cat,
		Title: title, Body: title + " body", Citations: cites}
}

// ISSUE-019: the four same-span pairs Opus 5.5 produced in the live
// TypeScript UAT (run 20261111T150647Z), each one problem reported twice:
// once by its nature, once as the ADR it violates. Both halves cite the ADR.
func TestFoldDuplicates_ObservedPairs(t *testing.T) {
	in := []Finding{
		f("src/calc.ts", 2, 2, schema.SeverityCritical, schema.CategorySecurity, "Remove eval of user-supplied formula", adrExec),
		f("src/calc.ts", 2, 2, schema.SeverityMajor, schema.CategoryArchitecture, "eval violates ADR-0001", adrExec),
		f("src/config.ts", 1, 1, schema.SeverityCritical, schema.CategorySecurity, "Remove hardcoded live payments token", adrEnvOnly),
		f("src/config.ts", 1, 1, schema.SeverityMajor, schema.CategoryArchitecture, "Secret literal violates ADR-0002", adrEnvOnly),
		f("src/export.ts", 4, 4, schema.SeverityCritical, schema.CategorySecurity, "Command injection via execSync string", adrExec),
		f("src/export.ts", 4, 4, schema.SeverityMajor, schema.CategoryArchitecture, "execSync with command string violates ADR-0001", adrExec),
		f("src/webhook.ts", 3, 3, schema.SeverityCritical, schema.CategorySecurity, "Empty-string fallback for HMAC secret", adrEnvOnly),
		f("src/webhook.ts", 3, 7, schema.SeverityMajor, schema.CategoryArchitecture, "Secret handling violates ADR-0002", adrEnvOnly),
	}
	got, merged := foldDuplicates(in)
	if merged != 4 {
		t.Fatalf("merged %d pairs, want 4", merged)
	}
	want := map[string]Finding{
		"src/calc.ts":    {StartLine: 2, EndLine: 2, Title: "Remove eval of user-supplied formula", Citations: []string{adrExec}},
		"src/config.ts":  {StartLine: 1, EndLine: 1, Title: "Remove hardcoded live payments token", Citations: []string{adrEnvOnly}},
		"src/export.ts":  {StartLine: 4, EndLine: 4, Title: "Command injection via execSync string", Citations: []string{adrExec}},
		"src/webhook.ts": {StartLine: 3, EndLine: 7, Title: "Empty-string fallback for HMAC secret", Citations: []string{adrEnvOnly}},
	}
	if len(got) != len(want) {
		t.Fatalf("%d findings after folding, want %d: %+v", len(got), len(want), got)
	}
	for _, g := range got {
		w, ok := want[g.File]
		if !ok {
			t.Fatalf("unexpected finding %+v", g)
		}
		// The rule decided in ISSUE-019: Architecture (so garmr VZ-002 still
		// sees the violated decision), the higher severity, the specific
		// problem's title, both bodies, the union of the spans and citations.
		if g.Category != schema.CategoryArchitecture || g.Severity != schema.SeverityCritical ||
			g.StartLine != w.StartLine || g.EndLine != w.EndLine || g.Title != w.Title ||
			!reflect.DeepEqual(g.Citations, w.Citations) {
			t.Errorf("%s: got %+v", g.File, g)
		}
		if g.Body != w.Title+" body\n\nViolates a documented design decision: "+archTitle(in, g.File)+" body" {
			t.Errorf("%s: body %q", g.File, g.Body)
		}
	}
}

func archTitle(in []Finding, file string) string {
	for i := range in {
		if in[i].File == file && in[i].Category == schema.CategoryArchitecture {
			return in[i].Title
		}
	}
	return ""
}

// Pairs that look alike but are different problems, or lack the shared
// citation that ties them to one decision, must stay separate.
func TestFoldDuplicates_KeepsDistinctFindings(t *testing.T) {
	for name, in := range map[string][]Finding{
		"different problem on the same line, no citation": {
			f("src/calc.ts", 2, 2, schema.SeverityCritical, schema.CategorySecurity, "eval of user input", adrExec),
			f("src/calc.ts", 2, 2, schema.SeverityMinor, schema.CategoryLogic, "unsafe return of any"),
		},
		"no Architecture half": {
			f("src/calc.ts", 2, 2, schema.SeverityCritical, schema.CategorySecurity, "eval of user input", adrExec),
			f("src/calc.ts", 2, 2, schema.SeverityMajor, schema.CategoryLogic, "result not validated", adrExec),
		},
		"both Architecture": {
			f("src/calc.ts", 2, 2, schema.SeverityMajor, schema.CategoryArchitecture, "eval violates ADR-0001", adrExec),
			f("src/calc.ts", 2, 2, schema.SeverityMajor, schema.CategoryArchitecture, "secret violates ADR-0002", adrEnvOnly),
		},
		"no shared citation": {
			f("src/webhook.ts", 3, 3, schema.SeverityCritical, schema.CategorySecurity, "timing leak", adrExec),
			f("src/webhook.ts", 3, 3, schema.SeverityMajor, schema.CategoryArchitecture, "secret handling", adrEnvOnly),
		},
		"non-Architecture half cites nothing": {
			f("src/config.ts", 1, 1, schema.SeverityCritical, schema.CategorySecurity, "hardcoded token"),
			f("src/config.ts", 1, 1, schema.SeverityMajor, schema.CategoryArchitecture, "secret literal", adrEnvOnly),
		},
		"lines do not overlap": {
			f("src/users.ts", 10, 12, schema.SeverityCritical, schema.CategorySecurity, "a", adrExec),
			f("src/users.ts", 13, 14, schema.SeverityMajor, schema.CategoryArchitecture, "b", adrExec),
		},
		"different files": {
			f("src/a.ts", 1, 1, schema.SeverityCritical, schema.CategorySecurity, "a", adrExec),
			f("src/b.ts", 1, 1, schema.SeverityMajor, schema.CategoryArchitecture, "b", adrExec),
		},
	} {
		got, merged := foldDuplicates(in)
		if merged != 0 || len(got) != len(in) {
			t.Errorf("%s: merged %d, %d findings left; want none merged", name, merged, len(got))
		}
	}
}

// One Architecture finding folds into at most one partner, and the choice
// does not depend on the order the model (or the chunks) produced them in.
func TestFoldDuplicates_DeterministicAndOneToOne(t *testing.T) {
	base := []Finding{
		f("src/export.ts", 4, 4, schema.SeverityCritical, schema.CategorySecurity, "Command injection via execSync string", adrExec),
		f("src/export.ts", 4, 4, schema.SeverityMajor, schema.CategoryLogic, "Report name not validated", adrExec),
		f("src/export.ts", 4, 4, schema.SeverityMajor, schema.CategoryArchitecture, "execSync violates ADR-0001", adrExec),
		f("src/calc.ts", 2, 2, schema.SeverityMinor, schema.CategoryMaintainability, "Name the formula type"),
	}
	want, merged := foldDuplicates(base)
	if merged != 1 || len(want) != 3 {
		t.Fatalf("merged %d, %d left; want 1 merged and 3 left: %+v", merged, len(want), want)
	}
	if want[0].Title != "Command injection via execSync string" || want[0].Category != schema.CategoryArchitecture {
		t.Errorf("the most severe partner must absorb the Architecture finding: %+v", want[0])
	}
	// Every ordering of the input, not a sample.
	for _, in := range permutations(base) {
		got, _ := foldDuplicates(in)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("order-dependent result for %v:\n got %+v\nwant %+v", titles(in), got, want)
		}
	}
}

func permutations(fs []Finding) [][]Finding {
	if len(fs) <= 1 {
		return [][]Finding{append([]Finding(nil), fs...)}
	}
	var out [][]Finding
	for i := range fs {
		rest := append(append([]Finding(nil), fs[:i]...), fs[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]Finding{fs[i]}, p...))
		}
	}
	return out
}

func titles(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for i := range fs {
		out = append(out, fs[i].Title)
	}
	return out
}

func TestFoldDuplicates_ResultValidates(t *testing.T) {
	got, _ := foldDuplicates([]Finding{
		f("src/calc.ts", 2, 2, schema.SeverityCritical, schema.CategorySecurity, "eval", adrExec),
		f("src/calc.ts", 2, 2, schema.SeverityMajor, schema.CategoryArchitecture, "eval violates ADR-0001", adrExec),
	})
	// An Architecture finding must carry citations (schema/review.cue).
	if len(got) != 1 || got[0].Category != schema.CategoryArchitecture || len(got[0].Citations) == 0 {
		t.Fatalf("merged finding %+v", got)
	}
}
