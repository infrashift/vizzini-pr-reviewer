package analysis

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"path"
	"strconv"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// OrderFilesByDependency orders a diff's files so that each Go file comes
// after the changed files it depends on: data models first, then the logic
// that uses them, then the UI on top (Milestone 4.3, for mrman's file tree).
//
// File A depends on file B when A imports B's package, or when A and B share
// a directory and A refers to a type B declares. Only the diff is used: new
// files are parsed in full with go/parser; for modified files the new-side
// hunk text is parsed if it forms a valid file, and otherwise scanned token
// by token, which still finds imports, type declarations and references.
//
// The order is Kahn's topological sort. Ties, and cycles (which Go allows
// within a package), are broken deterministically by path depth (shallower
// first), then path. Non-Go files follow the Go files in their diff order.
// The input slice is not modified.
func OrderFilesByDependency(files []*gitdiff.File) []*gitdiff.File {
	var nodes []*goFile
	var rest []*gitdiff.File
	for _, f := range files {
		p := filePath(f)
		if strings.HasSuffix(p, ".go") {
			nodes = append(nodes, inspect(p, newSideText(f)))
			nodes[len(nodes)-1].file = f
		} else {
			rest = append(rest, f)
		}
	}

	n := len(nodes)
	deps := make([]map[int]bool, n) // deps[a][b]: a depends on b
	for a := range nodes {
		deps[a] = map[int]bool{}
		for b := range nodes {
			if a != b && nodes[a].dependsOn(nodes[b]) {
				deps[a][b] = true
			}
		}
	}

	order := make([]*gitdiff.File, 0, len(files))
	done := make([]bool, n)
	for len(order) < n {
		next := -1
		// Prefer a file whose dependencies are all placed; on a cycle, take
		// the best remaining file regardless.
		for _, readyOnly := range []bool{true, false} {
			for i := range nodes {
				if done[i] || (readyOnly && !allDone(deps[i], done)) {
					continue
				}
				if next < 0 || nodes[i].less(nodes[next]) {
					next = i
				}
			}
			if next >= 0 {
				break
			}
		}
		done[next] = true
		order = append(order, nodes[next].file)
	}
	return append(order, rest...)
}

func allDone(deps map[int]bool, done []bool) bool {
	for b := range deps {
		if !done[b] {
			return false
		}
	}
	return true
}

func filePath(f *gitdiff.File) string {
	if f.IsDelete || f.NewName == "" {
		return f.OldName
	}
	return f.NewName
}

// newSideText reassembles the new-file side of every hunk (context and added
// lines). For a new file this is the whole file.
func newSideText(f *gitdiff.File) string {
	var b strings.Builder
	for _, frag := range f.TextFragments {
		for _, l := range frag.Lines {
			if l.Op != gitdiff.OpDelete {
				b.WriteString(l.Line)
			}
		}
	}
	return b.String()
}

// goFile is what one Go file contributes to the dependency graph.
type goFile struct {
	file     *gitdiff.File
	path     string
	dir      string
	imports  map[string]bool // import paths
	declares map[string]bool // type names declared in this file
	refs     map[string]bool // identifiers used in this file
}

func (g *goFile) depth() int { return strings.Count(g.path, "/") }

func (g *goFile) less(o *goFile) bool {
	return cmp.Or(cmp.Compare(g.depth(), o.depth()), cmp.Compare(g.path, o.path)) < 0
}

// dependsOn reports whether g needs o to be read first.
func (g *goFile) dependsOn(o *goFile) bool {
	if g.dir == o.dir {
		for name := range o.declares {
			if g.refs[name] && !g.declares[name] {
				return true
			}
		}
		return false
	}
	for imp := range g.imports {
		if importsDir(imp, o.dir) {
			return true
		}
	}
	return false
}

// importsDir reports whether import path imp names the package in the
// repository-relative directory dir: the module path is unknown, so a match
// on whole trailing path segments is used.
func importsDir(imp, dir string) bool {
	if dir == "." || dir == "" {
		return false
	}
	return imp == dir || strings.HasSuffix(imp, "/"+dir)
}

func inspect(p, src string) *goFile {
	g := &goFile{
		path: p, dir: path.Dir(p),
		imports: map[string]bool{}, declares: map[string]bool{}, refs: map[string]bool{},
	}
	fset := token.NewFileSet()
	if f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution); err == nil {
		g.fromAST(f)
	} else {
		g.fromTokens(src)
	}
	return g
}

func (g *goFile) fromAST(f *ast.File) {
	for _, imp := range f.Imports {
		if s, err := strconv.Unquote(imp.Path.Value); err == nil {
			g.imports[s] = true
		}
	}
	declIdents := map[*ast.Ident]bool{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			if ts, ok := s.(*ast.TypeSpec); ok {
				g.declares[ts.Name.Name] = true
				declIdents[ts.Name] = true
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ImportSpec:
			return false
		case *ast.Ident:
			if !declIdents[x] {
				g.refs[x.Name] = true
			}
		}
		return true
	})
	// The package clause's name is not a reference to anything.
	delete(g.refs, f.Name.Name)
}

// fromTokens handles hunk fragments that do not parse as a file: imports are
// string literals after "import" (including import blocks), a type
// declaration is an identifier after "type" or at the start of a line inside
// a "type ( ... )" block, and every other identifier is a reference.
func (g *goFile) fromTokens(src string) {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, []byte(src), nil, 0)

	var prev token.Token
	inImport, importBlock := false, false
	typeDepth := 0 // paren depth inside a "type (" block; 0 when outside
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		switch {
		case tok == token.IMPORT:
			inImport = true
		case inImport && tok == token.LPAREN:
			importBlock = true
		case inImport && tok == token.STRING:
			if v, err := strconv.Unquote(lit); err == nil {
				g.imports[v] = true
			}
			if !importBlock {
				inImport = false
			}
		case inImport && importBlock && tok == token.RPAREN:
			inImport, importBlock = false, false
		case tok == token.LPAREN && prev == token.TYPE:
			typeDepth = 1
		case typeDepth > 0 && tok == token.LPAREN:
			typeDepth++
		case typeDepth > 0 && tok == token.RPAREN:
			typeDepth--
		case tok == token.IDENT && (prev == token.TYPE || typeDepth == 1 && (prev == token.LPAREN || prev == token.SEMICOLON)):
			g.declares[lit] = true
		case tok == token.IDENT && !inImport:
			g.refs[lit] = true
		}
		prev = tok
	}
}
