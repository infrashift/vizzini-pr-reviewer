package analysis

import (
	"go/scanner"
	"go/token"
	"path"
	"strings"
)

// decisionPoints counts McCabe decision points on one changed line
// (ISSUE-020, ADR-0013). Go keeps an exact count from go/scanner: if, for,
// case, && and ||. C-family languages, Python and Ruby get the same metric
// from a small line lexer that drops string literals and comments and then
// counts whole-word decision keywords and operators. Any other file counts
// 0: its complexity is not estimated.
//
// The estimate is per line, so a keyword inside a string or comment that
// spans lines (a docstring, a block comment, a template literal) can count;
// lines that open or continue a /* block are skipped. The ternary ?:, ??
// and pattern-match arms are not counted: on one line, ? cannot be told
// apart from nullable types, ?. or generics.
func decisionPoints(file, line string) int {
	ext := strings.ToLower(path.Ext(file))
	if ext == ".go" {
		return goDecisionPoints(line)
	}
	if l := languages[ext]; l != nil {
		return l.count(line)
	}
	return 0
}

func goDecisionPoints(line string) int {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(line))
	s.Init(file, []byte(line), nil, 0)
	n := 0
	for {
		_, tok, _ := s.Scan()
		switch tok { //nolint:exhaustive // only decision tokens matter
		case token.EOF:
			return n
		case token.IF, token.FOR, token.CASE, token.LAND, token.LOR:
			n++
		}
	}
}

// lexRules describes one language family's decision points and the
// literal and comment syntax that hides them.
type lexRules struct {
	// keywords are whole-word decision points anywhere on the line.
	keywords map[string]bool
	// leading are decision points only as the line's first token (Python's
	// soft keyword case, which is also a common identifier).
	leading map[string]bool
	// boolOps counts && and ||.
	boolOps bool
	// slashComments: // to end of line and /* */ (a line that starts with
	// * or #, a block-comment continuation or a preprocessor line, counts 0).
	slashComments bool
	// hashComments: # to end of line.
	hashComments bool
	// singleQuote and backtick delimit string literals besides "...".
	singleQuote, backtick bool
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

var (
	cKeywords = set("if", "for", "foreach", "while", "case", "catch", "guard")
	// cFamily covers C, C++, C#, Java, Kotlin, Swift, PHP, Scala, Dart and
	// Groovy, where '...' is a char or string literal.
	cFamily = &lexRules{keywords: cKeywords, boolOps: true, slashComments: true, singleQuote: true}
	// jsFamily adds template literals.
	jsFamily = &lexRules{keywords: cKeywords, boolOps: true, slashComments: true, singleQuote: true, backtick: true}
	// rust has lifetimes ('a), so a single quote does not open a literal.
	rust   = &lexRules{keywords: cKeywords, boolOps: true, slashComments: true}
	python = &lexRules{
		keywords:     set("if", "elif", "for", "while", "except", "and", "or"),
		leading:      set("case"),
		hashComments: true, singleQuote: true,
	}
	ruby = &lexRules{
		keywords: set("if", "elsif", "unless", "while", "until", "for", "when", "rescue", "and", "or"),
		boolOps:  true, hashComments: true, singleQuote: true,
	}
)

// languages maps a lower-case file extension to its rules.
var languages = func() map[string]*lexRules {
	m := map[string]*lexRules{".rs": rust, ".py": python, ".pyi": python, ".rb": ruby}
	for _, e := range []string{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".cs", ".java", ".kt", ".kts",
		".swift", ".php", ".scala", ".dart", ".groovy"} {
		m[e] = cFamily
	}
	for _, e := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"} {
		m[e] = jsFamily
	}
	return m
}()

func (l *lexRules) count(line string) int {
	trimmed := strings.TrimSpace(line)
	if l.slashComments && (strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "#")) {
		return 0
	}
	n, first := 0, true
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
			continue
		case l.hashComments && c == '#':
			return n
		case l.slashComments && strings.HasPrefix(line[i:], "//"):
			return n
		case l.slashComments && strings.HasPrefix(line[i:], "/*"):
			end := strings.Index(line[i+2:], "*/")
			if end < 0 {
				return n
			}
			i += 2 + end + 2
		case c == '"' || (c == '\'' && l.singleQuote) || (c == '`' && l.backtick):
			end, ok := closeQuote(line, i)
			if !ok {
				return n // unterminated: the rest is inside the literal
			}
			i = end + 1
		case isWordByte(c):
			j := i
			for j < len(line) && (isWordByte(line[j]) || line[j] >= '0' && line[j] <= '9') {
				j++
			}
			w := line[i:j]
			if l.keywords[w] || (first && l.leading[w]) {
				n++
			}
			i = j
		case l.boolOps && (strings.HasPrefix(line[i:], "&&") || strings.HasPrefix(line[i:], "||")):
			n++
			i += 2
		default:
			i++
		}
		first = false
	}
	return n
}

// closeQuote finds the quote that closes the literal opened at line[open],
// skipping backslash escapes. It reports false for an unterminated literal.
func closeQuote(line string, open int) (int, bool) {
	q := line[open]
	for j := open + 1; j < len(line); j++ {
		switch line[j] {
		case '\\':
			j++
		case q:
			return j, true
		}
	}
	return 0, false
}

// isWordByte reports a byte that can start an identifier. Bytes of
// multi-byte UTF-8 runes count, so "naïve" stays one word.
func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
