package diff

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func parseFixture(t *testing.T, opts Options) *Diff {
	t.Helper()
	patch, err := os.ReadFile("testdata/multi.patch")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(bytes.NewReader(patch), opts)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

func paths(files []*File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func fileByPath(t *testing.T, d *Diff, p string) *File {
	t.Helper()
	for _, f := range d.Files {
		if f.Path == p {
			return f
		}
	}
	t.Fatalf("file %q not in %v", p, paths(d.Files))
	return nil
}

func TestParse_DefaultExcludes(t *testing.T) {
	d := parseFixture(t, Options{})

	wantKept := []string{"internal/app/service.go", "docs/README.md", "old.txt", "scripts/run.sh", "assets/logo.png"}
	if got := paths(d.Files); !reflect.DeepEqual(got, wantKept) {
		t.Errorf("kept files\n got %v\nwant %v", got, wantKept)
	}
	wantExcluded := []string{"go.sum", "vendor/github.com/x/y/y.go", "api/v1/types.pb.go", "web/yarn.lock", "internal/gql/models_gen.go"}
	if !reflect.DeepEqual(d.Excluded, wantExcluded) {
		t.Errorf("excluded\n got %v\nwant %v", d.Excluded, wantExcluded)
	}
}

func TestParse_CustomExcludes(t *testing.T) {
	tests := []struct {
		name     string
		exclude  []string
		wantKept int
		wantExcl []string
	}{
		{"empty slice excludes nothing", []string{}, 10, nil},
		{"anchored directory glob", []string{"docs/**"}, 9, []string{"docs/README.md"}},
		{"basename glob at any depth", []string{"*.png", "*.txt"}, 8, []string{"old.txt", "assets/logo.png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parseFixture(t, Options{Exclude: tt.exclude})
			if len(d.Files) != tt.wantKept {
				t.Errorf("kept %d files %v, want %d", len(d.Files), paths(d.Files), tt.wantKept)
			}
			if !reflect.DeepEqual(d.Excluded, tt.wantExcl) {
				t.Errorf("excluded %v, want %v", d.Excluded, tt.wantExcl)
			}
		})
	}
}

func TestParse_InvalidExcludePattern(t *testing.T) {
	_, err := Parse(strings.NewReader(""), Options{Exclude: []string{"["}})
	if err == nil {
		t.Fatal("want error for malformed glob, got nil")
	}
}

func TestParse_LineNumbers(t *testing.T) {
	f := fileByPath(t, parseFixture(t, Options{}), "internal/app/service.go")
	if len(f.Hunks) != 2 {
		t.Fatalf("got %d hunks, want 2", len(f.Hunks))
	}

	want := [][]Line{
		{
			{gitdiff.OpContext, 1, 1, "package app"},
			{gitdiff.OpAdd, 0, 2, `import "errors"`},
			{gitdiff.OpContext, 2, 3, "// Run runs."},
			{gitdiff.OpContext, 3, 4, "func Run() error {"},
		},
		{
			{gitdiff.OpContext, 10, 11, "    x := 1"},
			{gitdiff.OpDelete, 11, 0, "    return nil"},
			{gitdiff.OpAdd, 0, 12, `    return errors.New("boom")`},
			{gitdiff.OpContext, 12, 13, "}"},
		},
	}
	for i, h := range f.Hunks {
		if !reflect.DeepEqual(h.Lines, want[i]) {
			t.Errorf("hunk %d lines\n got %+v\nwant %+v", i, h.Lines, want[i])
		}
	}
}

func TestParse_AddedAndDeletedFiles(t *testing.T) {
	d := parseFixture(t, Options{})

	added := fileByPath(t, d, "docs/README.md")
	wantAdded := []Line{{gitdiff.OpAdd, 0, 1, "# Docs"}, {gitdiff.OpAdd, 0, 2, "Hello."}}
	if !reflect.DeepEqual(added.Hunks[0].Lines, wantAdded) {
		t.Errorf("added file lines %+v, want %+v", added.Hunks[0].Lines, wantAdded)
	}
	if got, want := added.Header(), "### docs/README.md (added)\n"; got != want {
		t.Errorf("header %q, want %q", got, want)
	}

	deleted := fileByPath(t, d, "old.txt")
	wantDeleted := []Line{{gitdiff.OpDelete, 1, 0, "bye"}}
	if !reflect.DeepEqual(deleted.Hunks[0].Lines, wantDeleted) {
		t.Errorf("deleted file lines %+v, want %+v", deleted.Hunks[0].Lines, wantDeleted)
	}
	if got, want := deleted.Header(), "### old.txt (deleted)\n"; got != want {
		t.Errorf("header %q, want %q", got, want)
	}
}

func TestParse_BinaryFile(t *testing.T) {
	f := fileByPath(t, parseFixture(t, Options{}), "assets/logo.png")
	if !f.Source.IsBinary {
		t.Error("IsBinary = false, want true")
	}
	if len(f.Hunks) != 0 {
		t.Errorf("binary file has %d hunks, want 0", len(f.Hunks))
	}
	if got, want := f.Header(), "### assets/logo.png (added, binary; content omitted)\n"; got != want {
		t.Errorf("header %q, want %q", got, want)
	}
}

func TestParse_RenameWithModeChange(t *testing.T) {
	f := fileByPath(t, parseFixture(t, Options{}), "scripts/run.sh")
	if len(f.Hunks) != 0 {
		t.Errorf("pure rename has %d hunks, want 0", len(f.Hunks))
	}
	want := "### scripts/run.sh (renamed from scripts/run, mode 100644 -> 100755)\n"
	if got := f.Header(); got != want {
		t.Errorf("header %q, want %q", got, want)
	}
}

func TestParse_Empty(t *testing.T) {
	d, err := Parse(strings.NewReader(""), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Files) != 0 || len(d.Excluded) != 0 {
		t.Errorf("got files %v excluded %v, want none", paths(d.Files), d.Excluded)
	}
}

func TestParse_MalformedHunk(t *testing.T) {
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,5 +1,5 @@\n a\n"
	if _, err := Parse(strings.NewReader(patch), Options{}); err == nil {
		t.Fatal("want error for hunk with miscounted lines, got nil")
	}
}

func TestHunkRender(t *testing.T) {
	f := fileByPath(t, parseFixture(t, Options{}), "internal/app/service.go")
	want := "@@ -10,3 +11,3 @@ func Run() error {\n" +
		"10 11       x := 1\n" +
		"11    -     return nil\n" +
		"   12 +     return errors.New(\"boom\")\n" +
		"12 13   }\n"
	if got := f.Hunks[1].Render(); got != want {
		t.Errorf("Render()\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"vendor/**", "vendor/a/b.go", true},
		{"vendor/**", "src/vendor/a.go", false},
		{"**/vendor/**", "src/vendor/a.go", true},
		{"**/vendor/**", "vendor/a.go", true},
		{"*.lock", "web/yarn.lock", true},
		{"*.lock", "lockfile.go", false},
		{"go.sum", "tools/go.sum", true},
		{"go.sum", "go.sum.bak", false},
		{"*_gen.go", "internal/gql/models_gen.go", true},
		{"*.pb.go", "api/v1/types.pb.go", true},
		{"*.pb.go", "api/v1/types.go", false},
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "docs/x/a.md", false},
	}
	for _, tt := range tests {
		got, err := matchGlob(tt.pattern, tt.name)
		if err != nil {
			t.Errorf("matchGlob(%q, %q): %v", tt.pattern, tt.name, err)
		}
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}
