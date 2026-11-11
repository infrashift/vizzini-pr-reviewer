package analysis

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

func diagStrings(ds []Diagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.String())
	}
	return out
}

func TestParseSARIF_RuffStyle(t *testing.T) {
	got, err := ParseSARIF(bytes.NewReader(readFixture(t, "sarif/ruff-style.sarif")), "/work/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"app/views.py:1:8 ruff: F401: `os` imported but unused",
		// No message text: the rule's shortDescription (by ruleIndex) is used.
		"app/views.py:5:12 ruff: S608: Possible SQL injection vector through string-based query construction",
	}
	if g := diagStrings(got); !reflect.DeepEqual(g, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestParseSARIF_RoslynStyleAndMultipleRuns(t *testing.T) {
	got, err := ParseSARIF(bytes.NewReader(readFixture(t, "sarif/roslyn-style.sarif")), "/work/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		// Absolute file:// URI relativized against the root, %20 decoded.
		"src/App/Users Repo.cs:14:9 Microsoft (R) Visual C# Compiler: CA2100: Review SQL queries for security vulnerabilities",
		// Relative URI resolved through originalUriBaseIds (SRCROOT).
		"src/App/Program.cs:7 Microsoft (R) Visual C# Compiler: CS0168: The variable 'ex' is declared but never used",
		// A suppression still under review is not in effect.
		"src/App/Program.cs:21 Microsoft (R) Visual C# Compiler: CA2100: suppression under review: kept",
		// A second run, from another tool.
		"tools/gen.py:2 semgrep: python.lang.security.audit.eval: Detected eval()",
	}
	if g := diagStrings(got); !reflect.DeepEqual(g, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestParseSARIF_WithoutRootDropsAbsoluteLocations(t *testing.T) {
	got, err := ParseSARIF(bytes.NewReader(readFixture(t, "sarif/roslyn-style.sarif")), "")
	if err != nil {
		t.Fatal(err)
	}
	if g := diagStrings(got); len(g) != 1 || !strings.HasPrefix(g[0], "tools/gen.py:2 semgrep") {
		t.Errorf("without a root only relative locations survive; got %v", g)
	}
}

func TestResolveURI(t *testing.T) {
	bases := map[string]sarifArtifactLocation{
		"SRC":    {URI: "file:///repo/src/"},
		"NESTED": {URI: "lib/", URIBaseID: "SRC"},
		"LOOP":   {URI: "x/", URIBaseID: "LOOP"},
	}
	tests := []struct {
		loc    sarifArtifactLocation
		root   string
		want   string
		wantOK bool
	}{
		{sarifArtifactLocation{URI: "a/b.py"}, "/repo", "a/b.py", true},
		{sarifArtifactLocation{URI: "file:///repo/a/b.py"}, "/repo/", "a/b.py", true},
		{sarifArtifactLocation{URI: "file://buildhost/repo/a.py"}, "/repo", "a.py", true},
		{sarifArtifactLocation{URI: "a.py", URIBaseID: "SRC"}, "/repo", "src/a.py", true},
		{sarifArtifactLocation{URI: "a.py", URIBaseID: "NESTED"}, "/repo", "src/lib/a.py", true},
		{sarifArtifactLocation{URI: "a.py", URIBaseID: "UNKNOWN"}, "/repo", "a.py", true},
		{sarifArtifactLocation{URI: "a.py", URIBaseID: "LOOP"}, "/repo", "", false},
		{sarifArtifactLocation{URI: "file:///C:/repo/src/A.cs"}, "C:/repo", "src/A.cs", true},
		{sarifArtifactLocation{URI: "file:///C:/repo/src/A.cs"}, `C:\repo`, "src/A.cs", true},
		{sarifArtifactLocation{URI: "file:///elsewhere/a.py"}, "/repo", "", false},
		{sarifArtifactLocation{URI: "https://example.com/a.py"}, "/repo", "", false},
		{sarifArtifactLocation{URI: "bad%zzescape.py"}, "/repo", "", false},
	}
	for _, tt := range tests {
		got, ok := resolveURI(tt.loc, bases, tt.root)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("resolveURI(%+v, %q) = %q, %v; want %q, %v", tt.loc, tt.root, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestParseLintReport_DetectsFormat(t *testing.T) {
	golangci, err := ParseLintReport(bytes.NewReader(readFixture(t, "golangci.json")), "/repo")
	if err != nil || len(golangci) != 5 || golangci[0].Linter != "gosec" {
		t.Errorf("golangci-lint: %d diags, err %v", len(golangci), err)
	}
	sarif, err := ParseLintReport(bytes.NewReader(readFixture(t, "sarif/ruff-style.sarif")), "/repo")
	if err != nil || len(sarif) != 2 || sarif[0].Linter != "ruff" {
		t.Errorf("SARIF: %d diags, err %v", len(sarif), err)
	}

	abs := `{"Issues":[{"FromLinter":"x","Text":"t","Pos":{"Filename":"/repo/pkg/a.go","Line":3}}]}` + "\n1 issues:\n* x: 1\n"
	got, err := ParseLintReport(strings.NewReader(abs), "/repo")
	if err != nil || len(got) != 1 || got[0].File != "pkg/a.go" {
		t.Errorf("absolute golangci path with trailing stats: %+v, %v", got, err)
	}

	for name, in := range map[string]string{
		"neither format": `{"results":[]}`,
		"not JSON":       `{`,
		"SARIF no runs":  `{"version":"2.1.0","runs":null}`,
	} {
		if _, err := ParseLintReport(strings.NewReader(in), "/repo"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// SARIF diagnostics go through the same changed-line filter as golangci-lint.
func TestOnChangedLines_SARIF(t *testing.T) {
	patch := "diff --git a/app/views.py b/app/views.py\n--- a/app/views.py\n+++ b/app/views.py\n@@ -1,4 +1,5 @@\n import json\n+q = \"SELECT * FROM t WHERE id = \" + uid\n x = 1\n y = 2\n z = 3\n"
	d, err := diff.Parse(strings.NewReader(patch), diff.Options{})
	if err != nil {
		t.Fatal(err)
	}
	diags := []Diagnostic{
		{Linter: "ruff", Text: "S608: SQL", File: "app/views.py", Line: 2},
		{Linter: "ruff", Text: "F401: unused", File: "app/views.py", Line: 1},
	}
	if got := diagStrings(OnChangedLines(diags, d.Files)); !reflect.DeepEqual(got, []string{"app/views.py:2 ruff: S608: SQL"}) {
		t.Errorf("got %v", got)
	}
}

// Captured from real ruff 0.17.4 by scripts/e2e-pylint.sh (SARIF_FIXTURE_OUT),
// with the temp repo path rewritten to file:///work/repo/.
func TestParseSARIF_RealRuff(t *testing.T) {
	got, err := ParseLintReport(bytes.NewReader(readFixture(t, "sarif/ruff-0.17.4.sarif")), "/work/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"app/views.py:1:8 ruff: F401: `json` imported but unused",
		"app/views.py:2:8 ruff: F401: `os` imported but unused",
		"app/views.py:6:12 ruff: S307: Use of possibly insecure function; consider using `ast.literal_eval`",
		"app/views.py:10:13 ruff: S608: Possible SQL injection vector through string-based query construction",
	}
	if g := diagStrings(got); !reflect.DeepEqual(g, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}
}

// Captured from real Roslyn (C# compiler 5.9.0, .NET SDK 10.0.404) by
// scripts/e2e-cslint.sh (SARIF_FIXTURE_OUT), with the temp repo path
// rewritten to /work/repo. Roslyn writes absolute file:// URIs and 331
// rule descriptors; results reference rules by ruleIndex.
func TestParseSARIF_RealRoslyn(t *testing.T) {
	got, err := ParseLintReport(bytes.NewReader(readFixture(t, "sarif/roslyn-5.9.0.sarif")), "/work/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"src/Users.cs:9:13 Microsoft (R) Visual C# Compiler: CS0168: The variable 'unused' is declared but never used",
		"src/Users.cs:14:13 Microsoft (R) Visual C# Compiler: CS0168: The variable 'retries' is declared but never used",
		"src/Users.cs:15:9 Microsoft (R) Visual C# Compiler: CA2100: Review if the query string passed to 'string DbCommand.CommandText' in 'Find', accepts any user input",
	}
	if g := diagStrings(got); !reflect.DeepEqual(g, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}

	// Without the matching root, absolute URIs cannot be placed and are dropped.
	none, err := ParseLintReport(bytes.NewReader(readFixture(t, "sarif/roslyn-5.9.0.sarif")), "/elsewhere")
	if err != nil || len(none) != 0 {
		t.Errorf("wrong root: %d diagnostics, err %v", len(none), err)
	}
}
