package analysis

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// newFile renders a unified diff that adds path with the given content.
func newFile(path, content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, path, len(lines))
	for _, l := range lines {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}

func parsePatch(t *testing.T, patch string) []*gitdiff.File {
	t.Helper()
	files, _, err := gitdiff.Parse(strings.NewReader(patch))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func order(files []*gitdiff.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range OrderFilesByDependency(files) {
		out = append(out, filePath(f))
	}
	return out
}

const (
	modelGo = `package model

type User struct {
	ID   int
	Name string
}
`
	serviceGo = `package service

import "example.com/app/model"

type Service struct{}

func (Service) Find(id int) model.User { return model.User{ID: id} }
`
	uiGo = `package ui

import (
	"fmt"

	"example.com/app/service"
)

func Render(s service.Service) string { return fmt.Sprint(s.Find(1)) }
`
)

// The milestone's acceptance test: ui -> service -> model, given in the
// worst order, comes back model first.
func TestOrder_ThreeLayers(t *testing.T) {
	patch := newFile("ui/ui.go", uiGo) + newFile("service/service.go", serviceGo) + newFile("model/model.go", modelGo)
	got := order(parsePatch(t, patch))
	want := []string{"model/model.go", "service/service.go", "ui/ui.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Every input permutation yields the same order.
func TestOrder_Deterministic(t *testing.T) {
	parts := []string{newFile("ui/ui.go", uiGo), newFile("service/service.go", serviceGo), newFile("model/model.go", modelGo), newFile("README.md", "# hi")}
	want := []string{"model/model.go", "service/service.go", "ui/ui.go", "README.md"}
	perms := [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {1, 3, 0, 2}, {2, 0, 3, 1}}
	for _, p := range perms {
		var patch strings.Builder
		for _, i := range p {
			patch.WriteString(parts[i])
		}
		if got := order(parsePatch(t, patch.String())); !reflect.DeepEqual(got, want) {
			t.Errorf("perm %v: got %v, want %v", p, got, want)
		}
	}
}

// Within one package, type references decide the order.
func TestOrder_SamePackageTypeReferences(t *testing.T) {
	patch := newFile("app/handler.go", "package app\n\ntype Handler struct{ svc *Service }\n") +
		newFile("app/service.go", "package app\n\ntype Service struct{ repo Repo }\n") +
		newFile("app/repo.go", "package app\n\ntype Repo interface{ Get(id int) string }\n")
	want := []string{"app/repo.go", "app/service.go", "app/handler.go"}
	if got := order(parsePatch(t, patch)); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A modified file's hunk is only a fragment: the token fallback still sees
// the reference, so the declaring file comes first.
func TestOrder_ModifiedFragment(t *testing.T) {
	fragment := "diff --git a/api/handler.go b/api/handler.go\n--- a/api/handler.go\n+++ b/api/handler.go\n" +
		"@@ -40,3 +40,4 @@ func (h *Handler) Get(w http.ResponseWriter) {\n" +
		" \tu := h.load()\n" +
		"+\tvar out store.Account\n" +
		" \twrite(w, u)\n" +
		" }\n"
	declares := "diff --git a/api/account.go b/api/account.go\n--- a/api/account.go\n+++ b/api/account.go\n" +
		"@@ -10,2 +10,6 @@ var x = 1\n" +
		" \n" +
		"+type (\n" +
		"+\tAccount struct{ ID int }\n" +
		"+\tUnused  func(a, b int) (int, error)\n" +
		"+)\n" +
		" var y = 2\n"
	got := order(parsePatch(t, fragment+declares))
	want := []string{"api/account.go", "api/handler.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFromTokens(t *testing.T) {
	g := inspect("x/a.go", "\timport (\n\t\"fmt\"\n\tdb \"example.com/app/db\"\n)\ntype T struct{}\ntype (\n\tA int\n\tB func(x int) (y int)\n)\nfunc f() { fmt.Println(Q) }\n")
	for _, imp := range []string{"fmt", "example.com/app/db"} {
		if !g.imports[imp] {
			t.Errorf("import %q not found: %v", imp, g.imports)
		}
	}
	for _, d := range []string{"T", "A", "B"} {
		if !g.declares[d] {
			t.Errorf("declaration %q not found: %v", d, g.declares)
		}
	}
	if g.declares["x"] || g.declares["y"] {
		t.Errorf("parameters mistaken for declarations: %v", g.declares)
	}
	if !g.refs["Q"] || !g.refs["Println"] {
		t.Errorf("references missing: %v", g.refs)
	}
}

// Go allows import-free cycles within a package; the shallower path wins.
func TestOrder_CycleBrokenByDepth(t *testing.T) {
	patch := newFile("pkg/deep/a.go", "package deep\n\ntype A struct{ b *B }\n") +
		newFile("pkg/deep/b.go", "package deep\n\ntype B struct{ a *A }\n")
	if got, want := order(parsePatch(t, patch)), []string{"pkg/deep/a.go", "pkg/deep/b.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same depth: got %v, want %v (path order)", got, want)
	}

	// A cycle across depths via a nested package directory sharing a type
	// name: the shallower file is placed first.
	patch = newFile("core/x.go", "package core\n\nimport \"example.com/app/core/sub\"\n\ntype X struct{ s sub.S }\n") +
		newFile("core/sub/s.go", "package sub\n\nimport \"example.com/app/core\"\n\ntype S struct{ x *core.X }\n")
	if got, want := order(parsePatch(t, patch)), []string{"core/x.go", "core/sub/s.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cross-depth cycle: got %v, want %v", got, want)
	}
}

func TestOrder_NonGoAndDeletedFiles(t *testing.T) {
	deleted := "diff --git a/old/legacy.go b/old/legacy.go\ndeleted file mode 100644\n--- a/old/legacy.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-package old\n-type Legacy struct{}\n"
	patch := newFile("docs/guide.md", "# Guide") + newFile("ui/ui.go", uiGo) + deleted + newFile("schema/review.cue", "package review") + newFile("model/model.go", modelGo)
	got := order(parsePatch(t, patch))
	want := []string{"model/model.go", "old/legacy.go", "ui/ui.go", "docs/guide.md", "schema/review.cue"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOrder_DoesNotModifyInput(t *testing.T) {
	files := parsePatch(t, newFile("ui/ui.go", uiGo)+newFile("model/model.go", modelGo))
	before := []*gitdiff.File{files[0], files[1]}
	_ = OrderFilesByDependency(files)
	if files[0] != before[0] || files[1] != before[1] {
		t.Error("input slice was reordered")
	}
	if got := OrderFilesByDependency(nil); len(got) != 0 {
		t.Errorf("nil input gave %v", got)
	}
}

func TestImportsDir(t *testing.T) {
	tests := []struct {
		imp, dir string
		want     bool
	}{
		{"example.com/app/model", "model", true},
		{"example.com/app/internal/model", "internal/model", true},
		{"example.com/app/supermodel", "model", false},
		{"model", "model", true},
		{"example.com/app", ".", false},
	}
	for _, tt := range tests {
		if got := importsDir(tt.imp, tt.dir); got != tt.want {
			t.Errorf("importsDir(%q, %q) = %v, want %v", tt.imp, tt.dir, got, tt.want)
		}
	}
}
