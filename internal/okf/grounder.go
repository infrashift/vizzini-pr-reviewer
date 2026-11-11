// Package okf loads an Open Knowledge Format (OKF) bundle, such as an okfw
// wiki's .okfw/ directory, and selects the documents relevant to a diff so
// reviews can cite design decisions (Milestone 4.5).
package okf

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueyaml "cuelang.org/go/encoding/yaml"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

// Doc is one OKF concept.
type Doc struct {
	// ID is the frontmatter id, or the bundle-relative path when absent.
	ID          string
	Title       string
	Description string
	// Path is relative to the bundle root, slash-separated.
	Path string
	// Paths holds repository globs the document governs (x.paths or paths).
	Paths []string
	// Domains holds package or directory names it governs (x.domain or domain).
	Domains []string
	Tags    []string
	// Body is the Markdown after the frontmatter.
	Body string
}

// Bundle is a loaded OKF directory.
type Bundle struct {
	Docs []Doc
}

// Match is a document selected for a diff, with why it was selected.
type Match struct {
	Doc *Doc
	// Reason is "paths", "domain" or "tags", strongest first.
	Reason string
}

var (
	// Markdown links in index.md listings: [Title](target) or [dir/](dir/).
	indexLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	// Go package clause inside hunk text.
	packageClause = regexp.MustCompile(`^\s*package\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

// Load reads a bundle by progressive disclosure: starting at the root
// index.md, it follows each listed concept and each listed subdirectory's
// index.md. A directory without index.md is scanned for *.md files instead.
// index.md and log.md are listings, not concepts, and are never loaded as
// documents. Concepts without frontmatter are skipped.
func Load(fsys fs.FS) (*Bundle, error) {
	l := &loader{fsys: fsys, ctx: cuecontext.New(), seen: map[string]bool{}}
	if err := l.dir("."); err != nil {
		return nil, err
	}
	slices.SortFunc(l.docs, func(a, b Doc) int { return cmp.Compare(a.Path, b.Path) })
	return &Bundle{Docs: l.docs}, nil
}

type loader struct {
	fsys fs.FS
	ctx  *cue.Context
	seen map[string]bool
	docs []Doc
}

func (l *loader) dir(dir string) error {
	if l.seen[dir+"/"] {
		return nil
	}
	l.seen[dir+"/"] = true

	index, err := fs.ReadFile(l.fsys, path.Join(dir, "index.md"))
	if errors.Is(err, fs.ErrNotExist) {
		return l.scan(dir)
	}
	if err != nil {
		return err
	}
	for _, m := range indexLink.FindAllStringSubmatch(string(index), -1) {
		target := m[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
			continue
		}
		target = strings.TrimPrefix(target, "/")
		if !strings.HasPrefix(m[1], "/") {
			target = path.Join(dir, target)
		}
		target = path.Clean(target)
		if strings.HasPrefix(target, "..") {
			continue
		}
		if strings.HasSuffix(m[1], "/") {
			if err := l.dir(target); err != nil {
				return err
			}
			continue
		}
		if err := l.file(target); err != nil {
			return err
		}
	}
	return nil
}

func (l *loader) scan(dir string) error {
	entries, err := fs.ReadDir(l.fsys, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := path.Join(dir, e.Name())
		switch {
		case strings.HasPrefix(e.Name(), "."):
		case e.IsDir():
			if err := l.dir(name); err != nil {
				return err
			}
		case strings.HasSuffix(e.Name(), ".md"):
			if err := l.file(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *loader) file(p string) error {
	base := path.Base(p)
	if l.seen[p] || !strings.HasSuffix(p, ".md") || base == "index.md" || base == "log.md" {
		return nil
	}
	l.seen[p] = true
	data, err := fs.ReadFile(l.fsys, p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // listings may point at concepts that no longer exist (OKF 6.1)
	}
	if err != nil {
		return err
	}
	fm, body, ok := splitFrontmatter(string(data))
	if !ok {
		return nil
	}
	d, err := l.parse(p, fm)
	if err != nil {
		return fmt.Errorf("okf: %s: %w", p, err)
	}
	d.Body = strings.TrimSpace(body)
	l.docs = append(l.docs, d)
	return nil
}

func splitFrontmatter(s string) (fm, body string, ok bool) {
	s = strings.TrimPrefix(s, "\ufeff")
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return "", "", false
	}
	rest := s[strings.IndexByte(s, '\n')+1:]
	for i := 0; i < len(rest); {
		nl := strings.IndexByte(rest[i:], '\n')
		line := rest[i:]
		if nl >= 0 {
			line = rest[i : i+nl]
		}
		if strings.TrimRight(line, "\r") == "---" {
			if nl < 0 {
				return rest[:i], "", true
			}
			return rest[:i], rest[i+nl+1:], true
		}
		if nl < 0 {
			break
		}
		i += nl + 1
	}
	return "", "", false
}

// parse decodes frontmatter with CUE's YAML encoder (no extra dependency).
// Governance rules are read from x.paths / x.domain (where okfw's closed
// schema puts custom keys) and from top-level paths / domain.
func (l *loader) parse(p, fm string) (Doc, error) {
	f, err := cueyaml.Extract(p, []byte(fm))
	if err != nil {
		return Doc{}, err
	}
	v := l.ctx.BuildFile(f)
	if err := v.Err(); err != nil {
		return Doc{}, err
	}
	d := Doc{
		ID:          str(v, "id"),
		Title:       str(v, "title"),
		Description: str(v, "description"),
		Path:        p,
		Tags:        list(v, "tags"),
		Paths:       append(list(v, "paths"), list(v, "x.paths")...),
		Domains:     append(list(v, "domain"), list(v, "x.domain")...),
	}
	if d.ID == "" {
		d.ID = p
	}
	if d.Title == "" {
		d.Title = strings.TrimSuffix(path.Base(p), ".md")
	}
	for _, g := range d.Paths {
		if _, err := diff.MatchGlob(g, "x"); err != nil {
			return Doc{}, fmt.Errorf("bad paths glob %q: %w", g, err)
		}
	}
	return d, nil
}

func str(v cue.Value, field string) string {
	s, err := v.LookupPath(cue.ParsePath(field)).String()
	if err != nil {
		return ""
	}
	return s
}

// list reads a string or a list of strings.
func list(v cue.Value, field string) []string {
	f := v.LookupPath(cue.ParsePath(field))
	if !f.Exists() {
		return nil
	}
	if s, err := f.String(); err == nil {
		return []string{s}
	}
	var out []string
	if err := f.Decode(&out); err != nil {
		return nil
	}
	return out
}

// Target is a changed file the documents are matched against.
type Target struct {
	Path string
	// Package is the Go package name if known; otherwise the directory name
	// stands in.
	Package string
}

// TargetsFor derives match targets from parsed diff files, reading the Go
// package clause from hunk text when it is visible.
func TargetsFor(files []*diff.File) []Target {
	out := make([]Target, 0, len(files))
	for _, f := range files {
		t := Target{Path: f.Path}
	scan:
		for i := range f.Hunks {
			for _, l := range f.Hunks[i].Lines {
				if m := packageClause.FindStringSubmatch(l.Text); m != nil {
					t.Package = m[1]
					break scan
				}
			}
		}
		out = append(out, t)
	}
	return out
}

// Match returns the documents relevant to the targets, strongest first:
// a paths glob matching a changed file, then a domain naming a changed
// file's package or directory, then a tag doing the same. Documents with
// no paths, domain or tags never match on their own.
func (b *Bundle) Match(targets []Target) []Match {
	var out []Match
	for i := range b.Docs {
		d := &b.Docs[i]
		if r := d.matchReason(targets); r != "" {
			out = append(out, Match{Doc: d, Reason: r})
		}
	}
	rank := map[string]int{"paths": 0, "domain": 1, "tags": 2}
	slices.SortStableFunc(out, func(a, c Match) int {
		return cmp.Or(cmp.Compare(rank[a.Reason], rank[c.Reason]), cmp.Compare(a.Doc.Path, c.Doc.Path))
	})
	return out
}

func (d *Doc) matchReason(targets []Target) string {
	for _, t := range targets {
		for _, g := range d.Paths {
			if ok, _ := diff.MatchGlob(g, t.Path); ok {
				return "paths"
			}
		}
	}
	if namesAny(d.Domains, targets) {
		return "domain"
	}
	if namesAny(d.Tags, targets) {
		return "tags"
	}
	return ""
}

// namesAny reports whether any term equals (case-insensitively) a changed
// file's package name or one of its directory names.
func namesAny(terms []string, targets []Target) bool {
	for _, term := range terms {
		for _, t := range targets {
			if t.Package != "" && strings.EqualFold(term, t.Package) {
				return true
			}
			segs := strings.Split(path.Dir(t.Path), "/")
			for _, s := range segs {
				if s != "." && strings.EqualFold(term, s) {
					return true
				}
			}
		}
	}
	return false
}
