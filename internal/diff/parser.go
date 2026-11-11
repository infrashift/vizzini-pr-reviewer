// Package diff parses unified diffs into files and hunks whose lines carry
// explicit old/new line numbers, so the LLM can anchor findings precisely.
package diff

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// DefaultExcludes lists generated or vendored paths that add noise to a review.
// A pattern without a slash matches the file's base name at any depth; a
// pattern with a slash matches the whole path from the repository root, where
// "**" matches zero or more path segments.
var DefaultExcludes = []string{"*.pb.go", "*_gen.go", "go.sum", "vendor/**", "*.lock"}

// Options configures Parse.
type Options struct {
	// Exclude holds glob patterns for paths to drop. Nil means DefaultExcludes;
	// a non-nil empty slice excludes nothing.
	Exclude []string
}

// Diff is a parsed patch.
type Diff struct {
	Files []*File
	// Excluded lists the paths dropped by the exclude globs, in patch order.
	Excluded []string
}

// File is one changed file.
type File struct {
	// Path is the new name, or the old name when the file was deleted.
	Path string
	// Source is the underlying parsed file, kept for later analysis stages.
	Source *gitdiff.File
	// Hunks is empty for binary files, mode-only changes and pure renames.
	Hunks []Hunk
}

// Hunk is one text fragment with line-number-annotated lines.
type Hunk struct {
	Fragment *gitdiff.TextFragment
	Lines    []Line
}

// Line is a single hunk line.
type Line struct {
	Op gitdiff.LineOp
	// OldLine is the line number in the old file, or 0 for an added line.
	OldLine int64
	// NewLine is the line number in the new file, or 0 for a deleted line.
	NewLine int64
	// Text is the line content without its trailing newline.
	Text string
}

// Parse reads a unified diff, drops excluded paths and annotates every hunk
// line with its old and new line numbers.
func Parse(r io.Reader, opts Options) (*Diff, error) {
	excludes := opts.Exclude
	if excludes == nil {
		excludes = DefaultExcludes
	}
	for _, p := range excludes {
		if _, err := matchGlob(p, "x"); err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", p, err)
		}
	}

	files, _, err := gitdiff.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	d := &Diff{Files: make([]*File, 0, len(files))}
	for _, gf := range files {
		p := filePath(gf)
		if excluded(p, excludes) {
			d.Excluded = append(d.Excluded, p)
			continue
		}
		f := &File{Path: p, Source: gf, Hunks: make([]Hunk, 0, len(gf.TextFragments))}
		for _, frag := range gf.TextFragments {
			f.Hunks = append(f.Hunks, annotate(frag))
		}
		d.Files = append(d.Files, f)
	}
	return d, nil
}

func filePath(f *gitdiff.File) string {
	if f.IsDelete || f.NewName == "" {
		return f.OldName
	}
	return f.NewName
}

func annotate(frag *gitdiff.TextFragment) Hunk {
	h := Hunk{Fragment: frag, Lines: make([]Line, 0, len(frag.Lines))}
	oldN, newN := frag.OldPosition, frag.NewPosition
	for _, l := range frag.Lines {
		line := Line{Op: l.Op, Text: strings.TrimSuffix(l.Line, "\n")}
		switch l.Op {
		case gitdiff.OpContext:
			line.OldLine, line.NewLine = oldN, newN
			oldN++
			newN++
		case gitdiff.OpDelete:
			line.OldLine = oldN
			oldN++
		case gitdiff.OpAdd:
			line.NewLine = newN
			newN++
		}
		h.Lines = append(h.Lines, line)
	}
	return h
}

func excluded(p string, patterns []string) bool {
	for _, pat := range patterns {
		// Patterns were validated in Parse, so the error is always nil here.
		if ok, _ := matchGlob(pat, p); ok {
			return true
		}
	}
	return false
}

// MatchGlob matches a slash-separated path against a pattern with the same
// semantics as the exclude globs (see DefaultExcludes). It is also used for
// risk path weights.
func MatchGlob(pattern, name string) (bool, error) {
	return matchGlob(pattern, name)
}

// matchGlob matches a slash-separated path against a pattern. See
// DefaultExcludes for the semantics.
func matchGlob(pattern, name string) (bool, error) {
	if !strings.Contains(pattern, "/") {
		return path.Match(pattern, path.Base(name))
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) (bool, error) {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			for i := 0; i <= len(name); i++ {
				ok, err := matchSegments(rest, name[i:])
				if ok || err != nil {
					return ok, err
				}
			}
			return false, nil
		}
		if len(name) == 0 {
			return false, nil
		}
		ok, err := path.Match(pat[0], name[0])
		if !ok || err != nil {
			return false, err
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0, nil
}

// Header renders a one-line description of the file for the LLM prompt.
// It always starts with "### ", so rendered pieces concatenate without
// changing how a BPE tokenizer splits them.
func (f *File) Header() string {
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(f.Path)
	b.WriteString(" (")
	src := f.Source
	switch {
	case src.IsNew:
		b.WriteString("added")
	case src.IsDelete:
		b.WriteString("deleted")
	case src.IsRename:
		b.WriteString("renamed from " + src.OldName)
	case src.IsCopy:
		b.WriteString("copied from " + src.OldName)
	default:
		b.WriteString("modified")
	}
	if src.OldMode != 0 && src.NewMode != 0 && src.OldMode != src.NewMode {
		fmt.Fprintf(&b, ", mode %o -> %o", src.OldMode, src.NewMode)
	}
	if src.IsBinary {
		b.WriteString(", binary; content omitted")
	}
	b.WriteString(")\n")
	return b.String()
}

// Render formats the hunk with its header and a line-number gutter:
//
//	@@ -10,3 +10,4 @@ func f()
//	10 10   unchanged line
//	11    - deleted line
//	   11 + added line
//
// It always starts with "@@" (see File.Header).
func (h *Hunk) Render() string {
	width := len(strconv.FormatInt(max(
		h.Fragment.OldPosition+h.Fragment.OldLines,
		h.Fragment.NewPosition+h.Fragment.NewLines,
	), 10))

	var b strings.Builder
	b.WriteString(h.Fragment.Header())
	b.WriteByte('\n')
	for _, l := range h.Lines {
		fmt.Fprintf(&b, "%*s %*s %c %s\n", width, lineNum(l.OldLine), width, lineNum(l.NewLine), opChar(l.Op), l.Text)
	}
	return b.String()
}

func lineNum(n int64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

func opChar(op gitdiff.LineOp) byte {
	switch op {
	case gitdiff.OpAdd:
		return '+'
	case gitdiff.OpDelete:
		return '-'
	default:
		return ' '
	}
}
