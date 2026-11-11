package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", filepath.Base(name)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseGolangciJSON(t *testing.T) {
	diags, err := ParseGolangciJSON(bytes.NewReader(readFixture(t, "golangci.json")))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 5 {
		t.Fatalf("got %d diagnostics, want 5", len(diags))
	}
	want := Diagnostic{Linter: "errcheck", Text: "Error return value of `r.ParseForm` is not checked", File: "internal/api/handler.go", Line: 4, Column: 13}
	if diags[3] != want {
		t.Errorf("diags[3] = %+v, want %+v (leading ./ normalized away)", diags[3], want)
	}
	if got := want.String(); got != "internal/api/handler.go:4:13 errcheck: Error return value of `r.ParseForm` is not checked" {
		t.Errorf("String() = %q", got)
	}
}

func TestParseGolangciJSON_EdgeCases(t *testing.T) {
	in := `{"Issues":[
		{"FromLinter":"x","Text":"no file","Pos":{"Filename":"","Line":3}},
		{"FromLinter":"x","Text":"no line","Pos":{"Filename":"a.go","Line":0}},
		{"FromLinter":"x","Text":" windows path ","Pos":{"Filename":"internal\\a\\b.go","Line":7}}
	]}`
	diags, err := ParseGolangciJSON(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []Diagnostic{{Linter: "x", Text: "windows path", File: "internal/a/b.go", Line: 7}}
	if !reflect.DeepEqual(diags, want) {
		t.Errorf("got %+v, want %+v", diags, want)
	}
	if got := want[0].String(); got != "internal/a/b.go:7 x: windows path" {
		t.Errorf("String() without column = %q", got)
	}

	if _, perr := ParseGolangciJSON(strings.NewReader(`{"Issues":[`)); perr == nil {
		t.Error("want error for malformed JSON")
	}
	empty, err := ParseGolangciJSON(strings.NewReader(`{"Issues":null}`))
	if err != nil || len(empty) != 0 {
		t.Errorf("null issues: %v, %v", empty, err)
	}
}

// golangci-lint v2 prints its per-linter stats ("2 issues:\n* errcheck: 2")
// to stdout after the JSON unless --show-stats=false is passed. Only the
// first JSON value is read, so that trailing text is harmless.
func TestParseGolangciJSON_TrailingStats(t *testing.T) {
	in := `{"Issues":[{"FromLinter":"errcheck","Text":"x","Pos":{"Filename":"a.go","Line":3,"Column":2}}],"Report":{}}` +
		"\n2 issues:\n* errcheck: 2\n"
	diags, err := ParseGolangciJSON(strings.NewReader(in))
	if err != nil {
		t.Fatalf("trailing stats text rejected: %v", err)
	}
	if len(diags) != 1 || diags[0].File != "a.go" {
		t.Errorf("got %+v", diags)
	}
}

// Only diagnostics on "+" lines of the diff survive: not context lines, not
// lines outside the hunks, not files the diff does not touch.
func TestOnChangedLines(t *testing.T) {
	d, err := diff.Parse(bytes.NewReader(readFixture(t, "change.patch")), diff.Options{})
	if err != nil {
		t.Fatal(err)
	}
	diags, err := ParseGolangciJSON(bytes.NewReader(readFixture(t, "golangci.json")))
	if err != nil {
		t.Fatal(err)
	}

	got := OnChangedLines(diags, d.Files)
	lines := make([]string, 0, len(got))
	for _, g := range got {
		lines = append(lines, g.String())
	}
	want := []string{
		"internal/api/handler.go:4:13 errcheck: Error return value of `r.ParseForm` is not checked",
		"internal/store/users.go:44:7 gosec: G202: SQL string concatenation",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(lines, "\n  "), strings.Join(want, "\n  "))
	}

	if out := OnChangedLines(nil, d.Files); len(out) != 0 {
		t.Errorf("no diagnostics in, %d out", len(out))
	}
}
