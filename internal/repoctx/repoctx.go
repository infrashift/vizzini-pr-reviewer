// Package repoctx finds code outside a diff that a reviewer needs to judge
// it: the definitions of symbols the change uses, and the call sites of
// symbols it changes (ISSUE-023, ADR-0014). It reads a repository through an
// fs.FS in one deterministic pass, with language-family patterns rather than
// parsers: good enough to help a model, not a language server.
package repoctx

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Family groups languages that share declaration and call syntax.
type Family string

// Language families. A file outside them is never read or matched.
const (
	Go      Family = "go"
	CFamily Family = "c"
	JS      Family = "js"
	Python  Family = "python"
	Ruby    Family = "ruby"
	Rust    Family = "rust"
)

var extFamily = func() map[string]Family {
	m := map[string]Family{".go": Go, ".py": Python, ".pyi": Python, ".rb": Ruby, ".rs": Rust}
	for _, e := range []string{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".cs", ".java", ".kt", ".kts",
		".swift", ".php", ".scala", ".dart", ".groovy"} {
		m[e] = CFamily
	}
	for _, e := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"} {
		m[e] = JS
	}
	return m
}()

func familyOf(p string) Family { return extFamily[strings.ToLower(path.Ext(p))] }

// FamilyOf reports the language family of a path, or "" when none applies.
func FamilyOf(p string) Family { return familyOf(p) }

// keywords never name a definition or a call.
var keywords = set("if", "else", "for", "foreach", "while", "do", "switch", "case", "catch", "try", "finally",
	"return", "new", "throw", "await", "async", "yield", "function", "func", "def", "class", "struct", "interface",
	"enum", "record", "type", "var", "let", "const", "using", "lock", "fixed", "typeof", "sizeof", "nameof", "default",
	"when", "match", "fn", "impl", "loop", "unless", "until", "elif", "and", "or", "not", "in", "is", "as", "with",
	"lambda", "super", "this", "self", "base", "go", "defer", "select", "range", "make", "import", "export", "from",
	"require", "include", "print", "echo", "assert", "where", "throws", "sealed", "public", "private", "protected",
	"static", "void", "int", "bool", "string", "char", "long", "double", "float", "object", "dynamic")

// commonCalls are names too generic to be worth looking up.
var commonCalls = set("len", "append", "cap", "copy", "delete", "panic", "recover", "close", "min", "max",
	"Println", "Printf", "Print", "Sprintf", "Errorf", "Fprintf", "Fatalf", "Logf", "log", "Log", "info", "warn",
	"error", "debug", "toString", "ToString", "String", "equals", "Equals", "hashCode", "GetHashCode", "get", "set",
	"add", "Add", "push", "pop", "map", "filter", "reduce", "then", "resolve", "reject", "keys", "values", "items",
	"length", "size", "has", "Has", "Contains", "contains", "join", "split", "trim", "format", "Format", "parse",
	"Parse", "Error", "Promise", "Object", "Array", "Number", "Boolean", "Date", "Math", "JSON", "isinstance",
	"str", "list", "dict", "tuple", "range", "enumerate", "zip", "sorted", "open", "super", "puts", "raise")

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// definition patterns per family; group 1 is the defined name.
var definitions = map[Family][]*regexp.Regexp{
	Go: {
		regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*[\[(]`),
		regexp.MustCompile(`^\s*type\s+([A-Za-z_]\w*)\b`),
	},
	CFamily: {
		regexp.MustCompile(`\b(?:class|interface|struct|record|enum|trait|object)\s+([A-Za-z_]\w*)`),
		// Method or function: modifiers and a return type, the name, a
		// parameter list, then a body, an expression body, or nothing (the
		// brace on the next line); never a statement ending in ;.
		regexp.MustCompile(`^\s*(?:[\w\[\]<>,.?:*&]+\s+)+([A-Za-z_]\w*)\s*\([^;]*\)\s*(?:\{.*|=>.*|throws\s.*|where\s.*|:\s*[\w\s,()]*\{?)?$`),
		regexp.MustCompile(`\bfunction\s+([A-Za-z_]\w*)\s*\(`),                        // PHP
		regexp.MustCompile(`\bfun\s+(?:<[^>]*>\s*)?(?:[\w.]+\.)?([A-Za-z_]\w*)\s*\(`), // Kotlin
		regexp.MustCompile(`\bfunc\s+([A-Za-z_]\w*)\s*[<(]`),                          // Swift
		regexp.MustCompile(`\bdef\s+([A-Za-z_]\w*)\s*[\[(:=]`),                        // Scala
	},
	JS: {
		regexp.MustCompile(`\bfunction\s*\*?\s+([A-Za-z_$][\w$]*)\s*[<(]`),
		regexp.MustCompile(`\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*(?::\s*[^=]+)?=>`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?:class|interface|enum|type)\s+([A-Za-z_$][\w$]*)`),
		regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|readonly|override|get|set)\s+)*([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*(?::\s*[^{=;]+)?\{\s*$`),
	},
	Python: {
		regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)\s*\(`),
		regexp.MustCompile(`^\s*class\s+([A-Za-z_]\w*)\b`),
	},
	Ruby: {
		regexp.MustCompile(`^\s*def\s+(?:self\.)?([A-Za-z_]\w*[?!=]?)`),
		regexp.MustCompile(`^\s*(?:class|module)\s+([A-Z]\w*)`),
	},
	Rust: {
		regexp.MustCompile(`\bfn\s+([A-Za-z_]\w*)\s*[<(]`),
		regexp.MustCompile(`\b(?:struct|enum|trait|type)\s+([A-Za-z_]\w*)`),
	},
}

// definedNames returns the names a line declares, in order, without
// duplicates. A line starting with a statement keyword declares nothing.
func definedNames(f Family, line string) []string {
	res := definitions[f]
	if len(res) == 0 {
		return nil
	}
	code := strip(f, line)
	if w := firstWord(code); w == "return" || w == "else" || w == "new" || w == "await" || w == "throw" || w == "if" || w == "for" || w == "while" {
		return nil
	}
	var out []string
	for _, re := range res {
		for _, m := range re.FindAllStringSubmatch(code, -1) {
			if n := m[1]; !keywords[n] && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}

var (
	callRe = regexp.MustCompile(`([A-Za-z_$][\w$]*[?!]?)\s*\(`)
	newRe  = regexp.MustCompile(`\bnew\s+([A-Za-z_$][\w$.]*?)\s*[<(]`)
)

// usedNames returns the names a line calls or instantiates, in order of
// first appearance, skipping keywords, very common calls, names shorter
// than three characters and names the line itself defines.
func usedNames(f Family, line string) []string {
	if definitions[f] == nil {
		return nil
	}
	code := strip(f, line)
	defined := definedNames(f, line)
	type hit struct {
		at   int
		name string
	}
	var hits []hit
	for _, m := range newRe.FindAllStringSubmatchIndex(code, -1) {
		name := code[m[2]:m[3]]
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		hits = append(hits, hit{m[2], name})
	}
	for _, m := range callRe.FindAllStringSubmatchIndex(code, -1) {
		hits = append(hits, hit{m[2], code[m[2]:m[3]]})
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return a.at - b.at })
	var out []string
	for _, h := range hits {
		n := strings.TrimRight(h.name, "?!")
		if len(n) < 3 || keywords[n] || commonCalls[n] || slices.Contains(defined, n) || slices.Contains(out, n) {
			continue
		}
		out = append(out, n)
	}
	return out
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, func(r rune) bool {
		return r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if i < 0 {
		return s
	}
	return s[:i]
}

// strip blanks out string literals and comments so their words are not
// mistaken for code. Positions are kept (contents become spaces).
func strip(f Family, line string) string {
	b := []byte(line)
	hash := f == Python || f == Ruby
	slash := !hash
	single := f != Rust
	back := f == JS
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case hash && c == '#', slash && c == '/' && i+1 < len(b) && b[i+1] == '/':
			for j := i; j < len(b); j++ {
				b[j] = ' '
			}
			return string(b)
		case slash && c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(string(b[i+2:]), "*/")
			stop := len(b)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			for j := i; j < stop; j++ {
				b[j] = ' '
			}
			i = stop - 1
		case c == '"' || (c == '\'' && single) || (c == '`' && back):
			j := i + 1
			for ; j < len(b) && b[j] != c; j++ {
				if b[j] == '\\' {
					b[j] = ' '
					j++
					if j < len(b) {
						b[j] = ' '
					}
					continue
				}
				b[j] = ' '
			}
			i = j
		}
	}
	return string(b)
}

// Want lists the symbols to look for, per language family.
type Want struct {
	// Used symbols get their definitions found.
	Used map[Family]map[string]bool
	// Defined symbols get their call sites found.
	Defined map[Family]map[string]bool
}

// Limits bound a scan. Zero values select the defaults.
type Limits struct {
	// MaxFiles caps how many source files are read (default 20000).
	MaxFiles int
	// MaxFileBytes skips larger files (default 512 KiB).
	MaxFileBytes int
	// MaxTotalBytes stops the scan once this much has been read (default 64 MiB).
	MaxTotalBytes int
	// Exclude holds globs (the diff's excludes) for paths never to read.
	Exclude []string
	// MatchGlob reports whether a glob matches a path; nil uses path.Match
	// on the base name and the whole path.
	MatchGlob func(glob, p string) bool
}

// Snippet is one region of a file given to the model as context.
type Snippet struct {
	Path string
	// Start and End are 1-based, inclusive line numbers.
	Start, End int
	// Kind is "definition" or "caller".
	Kind   string
	Symbol string
	Family Family
	// Text holds the lines Start..End.
	Text string
}

const (
	callerRadius  = 2
	maxDefinition = 30
)

// skipDirs are never descended into.
var skipDirs = set(".git", ".hg", ".svn", "vendor", "node_modules", "bower_components", "dist", "build", "out",
	"target", "bin", "obj", ".venv", "venv", "__pycache__", ".next", ".gradle", ".idea", ".vs")

// Scan reads fsys once and returns the definitions of want.Used symbols and
// the call sites of want.Defined symbols, in path then line order. Only
// files of a family with wanted symbols are read.
func Scan(fsys fs.FS, want Want, lim Limits) ([]Snippet, error) {
	lim = lim.withDefaults()
	var out []Snippet
	files, total := 0, 0
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		if d.IsDir() {
			if p != "." && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		fam := familyOf(p)
		if fam == "" || (len(want.Used[fam]) == 0 && len(want.Defined[fam]) == 0) || lim.excluded(p) {
			return nil
		}
		if files >= lim.MaxFiles || total >= lim.MaxTotalBytes {
			return fs.SkipAll
		}
		info, ierr := d.Info()
		if ierr != nil || info.Size() > int64(lim.MaxFileBytes) {
			return nil //nolint:nilerr // skipped, not fatal
		}
		data, rerr := fs.ReadFile(fsys, p)
		if rerr != nil || slices.Contains(data, 0) {
			return nil //nolint:nilerr // unreadable or binary: skipped
		}
		files++
		total += len(data)
		out = append(out, scanFile(p, fam, string(data), want.Used[fam], want.Defined[fam])...)
		return nil
	})
	return out, err
}

func scanFile(p string, fam Family, text string, used, defined map[string]bool) []Snippet {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var out []Snippet
	for i, line := range lines {
		if !containsAny(line, used) && !containsAny(line, defined) {
			continue
		}
		defs := definedNames(fam, line)
		for _, n := range defs {
			if used[n] {
				end := definitionEnd(fam, lines, i)
				out = append(out, snippet(p, fam, "definition", n, lines, i, end))
			}
		}
		for _, n := range usedNames(fam, line) {
			if defined[n] && !slices.Contains(defs, n) {
				out = append(out, snippet(p, fam, "caller", n, lines, max(i-callerRadius, 0), min(i+callerRadius, len(lines)-1)))
			}
		}
	}
	return out
}

func containsAny(line string, names map[string]bool) bool {
	for n := range names {
		if strings.Contains(line, n) {
			return true
		}
	}
	return false
}

func snippet(p string, fam Family, kind, sym string, lines []string, from, to int) Snippet {
	return Snippet{Path: p, Start: from + 1, End: to + 1, Kind: kind, Symbol: sym, Family: fam,
		Text: strings.Join(lines[from:to+1], "\n")}
}

// definitionEnd finds the last line of the definition starting at line i:
// the line that closes its braces, the end of an indented block, or a
// statement ending in ";" for a body-less declaration; at most
// maxDefinition lines.
func definitionEnd(fam Family, lines []string, i int) int {
	last := min(i+maxDefinition-1, len(lines)-1)
	if fam == Python || fam == Ruby {
		ind := indent(lines[i])
		for j := i + 1; j <= last; j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" {
				continue
			}
			if indent(lines[j]) <= ind {
				if fam == Ruby && t == "end" {
					return j
				}
				return prevNonBlank(lines, j-1, i)
			}
		}
		return last
	}
	depth, opened := 0, false
	for j := i; j <= last; j++ {
		code := strip(fam, lines[j])
		depth += strings.Count(code, "{") - strings.Count(code, "}")
		if strings.Contains(code, "{") {
			opened = true
		}
		if opened && depth <= 0 {
			return j
		}
		if !opened && strings.HasSuffix(strings.TrimSpace(code), ";") {
			return j
		}
	}
	return last
}

func prevNonBlank(lines []string, j, floor int) int {
	for ; j > floor && strings.TrimSpace(lines[j]) == ""; j-- {
	}
	return j
}

func indent(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }

func (l Limits) withDefaults() Limits {
	if l.MaxFiles <= 0 {
		l.MaxFiles = 20000
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = 512 << 10
	}
	if l.MaxTotalBytes <= 0 {
		l.MaxTotalBytes = 64 << 20
	}
	if l.MatchGlob == nil {
		l.MatchGlob = func(g, p string) bool {
			if ok, _ := path.Match(g, p); ok {
				return true
			}
			ok, _ := path.Match(g, path.Base(p))
			return ok
		}
	}
	return l
}

func (l Limits) excluded(p string) bool {
	for _, g := range l.Exclude {
		if l.MatchGlob(g, p) {
			return true
		}
	}
	return false
}

// Symbols extracts what one changed file's hunks use and define: used from
// added lines, defined from added or deleted lines (a changed contract
// matters to callers either way).
func Symbols(p string, added, deleted []string) (fam Family, used, defined []string) {
	fam = familyOf(p)
	if fam == "" {
		return "", nil, nil
	}
	for _, l := range added {
		for _, n := range usedNames(fam, l) {
			if !slices.Contains(used, n) {
				used = append(used, n)
			}
		}
	}
	for _, l := range slices.Concat(added, deleted) {
		for _, n := range definedNames(fam, l) {
			if !slices.Contains(defined, n) {
				defined = append(defined, n)
			}
		}
	}
	// A symbol defined in the same change shows its own definition.
	used = slices.DeleteFunc(used, func(n string) bool { return slices.Contains(defined, n) })
	return fam, used, defined
}
