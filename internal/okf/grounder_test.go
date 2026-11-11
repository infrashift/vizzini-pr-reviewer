package okf

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

func loadFixture(t *testing.T) *Bundle {
	t.Helper()
	b, err := Load(os.DirFS("testdata/okf"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ids(docs []Doc) []string {
	out := make([]string, 0, len(docs))
	for i := range docs {
		out = append(out, docs[i].ID)
	}
	return out
}

func TestLoad_ProgressiveDisclosure(t *testing.T) {
	b := loadFixture(t)
	// index-listed concepts, an index-less directory scanned, the orphan
	// (unlisted) skipped, the dangling "gone.md" tolerated, index.md never a doc.
	want := []string{"conv-0001", "adr-0002-api", "adr-0001-store", "note-store-perf"}
	if got := ids(b.Docs); !reflect.DeepEqual(got, want) {
		t.Errorf("loaded %v, want %v", got, want)
	}
}

func TestLoad_Frontmatter(t *testing.T) {
	b := loadFixture(t)
	byID := map[string]Doc{}
	for _, d := range b.Docs {
		byID[d.ID] = d
	}
	store := byID["adr-0001-store"]
	if !reflect.DeepEqual(store.Paths, []string{"internal/store/**"}) || !reflect.DeepEqual(store.Domains, []string{"store"}) {
		t.Errorf("x.paths / x.domain not read: %+v", store)
	}
	if store.Title != "ADR-0001: Parameterized SQL only" || store.Path != "decisions/adr-store.md" || !strings.Contains(store.Body, "string concatenation is forbidden") {
		t.Errorf("store doc %+v", store)
	}
	if strings.HasPrefix(store.Body, "---") {
		t.Error("frontmatter leaked into the body")
	}
	if api := byID["adr-0002-api"]; !reflect.DeepEqual(api.Paths, []string{"internal/api/**"}) {
		t.Errorf("top-level paths not read: %+v", api.Paths)
	}
	if note := byID["note-store-perf"]; !reflect.DeepEqual(note.Domains, []string{"store", "cache"}) {
		t.Errorf("domain list not read: %+v", note.Domains)
	}
}

func targets(paths ...string) []Target {
	out := make([]Target, 0, len(paths))
	for _, p := range paths {
		out = append(out, Target{Path: p})
	}
	return out
}

func matchIDs(ms []Match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Doc.ID+"/"+m.Reason)
	}
	return out
}

// The milestone's acceptance test: only documents relevant to the changed
// paths are selected.
func TestMatch_SelectiveByDiffPaths(t *testing.T) {
	b := loadFixture(t)
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{"store change", []string{"internal/store/users.go"}, []string{"adr-0001-store/paths", "note-store-perf/domain"}},
		{"api change", []string{"internal/api/handler.go"}, []string{"adr-0002-api/paths"}},
		{"both", []string{"internal/api/h.go", "internal/store/u.go"}, []string{"adr-0002-api/paths", "adr-0001-store/paths", "note-store-perf/domain"}},
		{"cache dir via domain list", []string{"pkg/cache/lru.go"}, []string{"note-store-perf/domain"}},
		{"docs only", []string{"README.md", "docs/guide.md"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchIDs(b.Match(targets(tt.paths...))); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_PackageNameAndTags(t *testing.T) {
	b := loadFixture(t)
	// A Go package named "store" outside a store/ directory still matches the domain.
	got := matchIDs(b.Match([]Target{{Path: "pkg/db/users.go", Package: "store"}}))
	if want := []string{"adr-0001-store/domain", "note-store-perf/domain"}; !reflect.DeepEqual(got, want) {
		t.Errorf("package match %v, want %v", got, want)
	}
	// Tags match directory names too, ranked after paths and domain.
	got = matchIDs(b.Match(targets("internal/conventions/style.go")))
	if want := []string{"conv-0001/tags"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tag match %v, want %v", got, want)
	}
}

func TestTargetsFor_ReadsPackageClause(t *testing.T) {
	patch := "diff --git a/pkg/db/users.go b/pkg/db/users.go\n--- a/pkg/db/users.go\n+++ b/pkg/db/users.go\n@@ -1,2 +1,3 @@\n package store\n+// x\n import \"x\"\n" +
		"diff --git a/web/app.ts b/web/app.ts\n--- a/web/app.ts\n+++ b/web/app.ts\n@@ -5 +5 @@\n-a\n+b\n"
	d, err := diff.Parse(strings.NewReader(patch), diff.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Target{{Path: "pkg/db/users.go", Package: "store"}, {Path: "web/app.ts"}}
	if got := TargetsFor(d.Files); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoad_EdgeCases(t *testing.T) {
	t.Run("no index anywhere, no frontmatter, dotfiles", func(t *testing.T) {
		b, err := Load(fstest.MapFS{
			"a.md":         {Data: []byte("---\ntitle: A\ntype: Note\n---\nbody\n")},
			"plain.md":     {Data: []byte("# just markdown\n")},
			".hidden/x.md": {Data: []byte("---\nid: hidden\n---\n")},
			"sub/b.md":     {Data: []byte("---\r\nid: b\r\ntype: Note\r\n---\r\nwindows\r\n")},
			"log.md":       {Data: []byte("---\nid: log\n---\n")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(b.Docs); !reflect.DeepEqual(got, []string{"a.md", "b"}) {
			t.Errorf("loaded %v, want [a.md b] (id falls back to path)", got)
		}
		if b.Docs[0].Title != "A" {
			t.Errorf("title %q", b.Docs[0].Title)
		}
	})

	t.Run("malformed YAML is an error", func(t *testing.T) {
		_, err := Load(fstest.MapFS{"index.md": {Data: []byte("* [x](x.md)\n")}, "x.md": {Data: []byte("---\nid: [unclosed\n---\n")}})
		if err == nil || !strings.Contains(err.Error(), "x.md") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("bad paths glob is an error", func(t *testing.T) {
		_, err := Load(fstest.MapFS{"x.md": {Data: []byte("---\nid: x\npaths: [\"[\"]\n---\n")}})
		if err == nil || !strings.Contains(err.Error(), "bad paths glob") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("index links cannot escape the bundle", func(t *testing.T) {
		b, err := Load(fstest.MapFS{"index.md": {Data: []byte("* [up](../secret.md)\n* [web](https://example.com/a.md)\n")}})
		if err != nil || len(b.Docs) != 0 {
			t.Errorf("docs %v err %v", b, err)
		}
	})
}

// This repository's own wiki is a real okfw bundle; it must load cleanly.
func TestLoad_ThisRepositoryWiki(t *testing.T) {
	b, err := Load(os.DirFS("../../.okfw"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Docs) < 20 {
		t.Errorf("loaded only %d concepts from .okfw", len(b.Docs))
	}
}
