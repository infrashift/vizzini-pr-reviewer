package analysis

import "testing"

// ISSUE-020: decision points per changed line, for every language family the
// risk score estimates. Each case is one line of source.
func TestDecisionPoints(t *testing.T) {
	for _, tc := range []struct {
		path, line string
		want       int
	}{
		// Go keeps the exact go/scanner count.
		{"a.go", "if a && b || c {", 3},
		{"a.go", `s := "if && ||" // for case`, 0},
		{"a.go", "for _, x := range xs {", 1},

		// C-family: C#, Java, JavaScript/TypeScript, C/C++, Kotlin, Swift, Rust, PHP, ...
		{"src/Users.cs", "if (user != null && user.IsAdmin)", 2},
		{"src/Users.cs", "} else if (count > 0 || force) {", 2},
		{"src/Users.cs", "foreach (var row in rows)", 1},
		{"src/Users.cs", "} catch (SqlException ex) when (ex.Number == 1205) {", 1},
		{"src/Users.cs", `cmd.CommandText = "SELECT * FROM t WHERE a = 1 && b = 2 if";`, 0},
		{"src/Users.cs", `var verbatim = @"C:\path\if\for";`, 0},
		{"src/Users.cs", "// if this ever fails, retry while (true)", 0},
		{"src/Users.cs", "x = a; /* if && */ if (b) { }", 1},
		{"src/Users.cs", "  * if the caller passes null, case 3 applies", 0},
		{"src/Users.cs", "/** while documenting for loops */", 0},
		{"src/Users.cs", "var iffy = format(orders, forecast, android);", 0},
		{"src/Users.cs", "switch (kind) { case Kind.A: case Kind.B: break; }", 2},
		{"src/Users.cs", "char c = '\\''; if (c == '{') {", 1},
		{"Main.java", "while (it.hasNext() && !done) {", 2},
		{"app.ts", "for (const u of users) if (u.admin) count++;", 2},
		{"app.ts", "const msg = `retry ${n} times if && || fails`;", 0},
		{"app.ts", "const re = 'if (x) or && for'; return a || b;", 1},
		{"app.ts", "try { run(); } catch { return undefined; }", 1},
		{"app.ts", "const v = cond ? a : b ?? c;", 0},
		{"app.js", "if (a) { while (b) {} }", 2},
		{"lib.rs", "fn first<'a>(xs: &'a [u8]) -> Option<&'a u8> { if xs.is_empty() || x { None } }", 2},
		{"lib.rs", `let s = "if && ||";`, 0},
		{"View.swift", "guard let user = user, user.active else { return }", 1},
		{"index.php", "if ($a && $b) { foreach ($xs as $x) {} }", 3},

		// Python.
		{"app.py", "if a and b or c:", 3},
		{"app.py", "elif not ready:", 1},
		{"app.py", "x = [y for y in ys if y]", 2},
		{"app.py", "except (KeyError, ValueError):", 1},
		{"app.py", "case Point(x=0):", 1},
		{"app.py", "showcase = case_id  # case if and or", 0},
		{"app.py", `msg = "if this and that or else"`, 0},
		{"app.py", "s = 'for x in y' if flag else ''", 1},
		{"app.py", "while retries and not done:", 2},
		{"app.py", "ordered = android or iffy", 1},

		// Ruby.
		{"app.rb", "return unless user && user.admin?", 2},
		{"app.rb", "elsif count > 0 or force", 2},
		{"app.rb", "rescue ActiveRecord::RecordNotFound => e", 1},
		{"app.rb", "when :admin then grant", 1},
		{"app.rb", `puts "if unless" # while`, 0},

		// Languages without rules stay 0.
		{"config.yaml", "if: true  # and or", 0},
		{"README.md", "if you can, for example && || case", 0},
		{"schema.sql", "SELECT CASE WHEN a AND b THEN 1 END", 0},
		{"Makefile", "if [ -f x ] && [ -f y ]; then", 0},
	} {
		if got := decisionPoints(tc.path, tc.line); got != tc.want {
			t.Errorf("%s: %q = %d, want %d", tc.path, tc.line, got, tc.want)
		}
	}
}

// A line the lexer cannot close (an unterminated string, as when a literal
// spans lines) must not panic and must not count what follows the quote.
func TestDecisionPoints_UnterminatedLiterals(t *testing.T) {
	for _, tc := range []struct {
		path, line string
		want       int
	}{
		{"app.ts", "if (a) { const s = `starts a template if && ||", 1},
		{"app.py", `if x: s = """docstring if and or`, 1},
		{"src/A.cs", `if (a) Log("unterminated if &&`, 1},
		{"src/A.cs", "x = 1; /* opens a block if && ||", 0},
	} {
		if got := decisionPoints(tc.path, tc.line); got != tc.want {
			t.Errorf("%s: %q = %d, want %d", tc.path, tc.line, got, tc.want)
		}
	}
}
