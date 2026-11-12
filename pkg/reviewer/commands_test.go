package reviewer

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		in   string
		want Command
	}{
		{"/explain internal/store/users.go:44-45", Command{Kind: CommandExplain, File: "internal/store/users.go", StartLine: 44, EndLine: 45}},
		{"/explain a.go:7", Command{Kind: CommandExplain, File: "a.go", StartLine: 7, EndLine: 7}},
		{"  /EXPLAIN a.go:L3-L9   why is this   safe? ", Command{Kind: CommandExplain, File: "a.go", StartLine: 3, EndLine: 9, Question: "why is this safe?"}},
		{"/refactor pkg/x.go:10-20 extract a helper", Command{Kind: CommandRefactor, File: "pkg/x.go", StartLine: 10, EndLine: 20, Question: "extract a helper"}},
		{"/docstrings internal/api/handler.go", Command{Kind: CommandDocstrings, File: "internal/api/handler.go"}},
		{"/docstrings dir/odd:name.go public API only", Command{Kind: CommandDocstrings, File: "dir/odd:name.go", Question: "public API only"}},
	}
	for _, tt := range tests {
		got, err := ParseCommand(tt.in)
		if err != nil {
			t.Errorf("ParseCommand(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(*got, tt.want) {
			t.Errorf("ParseCommand(%q) = %+v, want %+v", tt.in, *got, tt.want)
		}
	}
}

func TestParseCommand_Errors(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "commands start with",
		"explain a.go:1":       "commands start with",
		"/summarize a.go:1":    "unknown command /summarize",
		"/explain":             "/explain needs a target",
		"/explain a.go":        "needs FILE:LINE or FILE:START-END",
		"/refactor a.go:9-3":   "range ends (3) before it starts (9)",
		"/refactor a.go:0":     `bad line number "0"`,
		"/explain a.go:12-x":   `bad line number "x"`,
		"/docstrings a.go:4-5": "/docstrings takes a file, not a line range",
	} {
		_, err := ParseCommand(in)
		if !errors.Is(err, ErrBadCommand) || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseCommand(%q) err = %v, want ErrBadCommand mentioning %q", in, err, want)
		}
	}
}

func TestCommandString(t *testing.T) {
	for _, c := range []struct {
		cmd  Command
		want string
	}{
		{Command{File: "a.go", StartLine: 3, EndLine: 9}, "a.go:3-9"},
		{Command{File: "a.go", StartLine: 3, EndLine: 3}, "a.go:3"},
		{Command{File: "a.go"}, "a.go"},
	} {
		if got := c.cmd.String(); got != c.want {
			t.Errorf("%+v.String() = %q, want %q", c.cmd, got, c.want)
		}
	}
}

// Two hunks in one file, so scoping can be observed.
const twoHunks = `diff --git a/internal/store/users.go b/internal/store/users.go
--- a/internal/store/users.go
+++ b/internal/store/users.go
@@ -10,3 +10,4 @@ func Open() {
 	db := connect()
+	db.SetMaxOpenConns(10)
 	return db
 }
@@ -41,6 +42,7 @@ func (s *Store) FindUser(ctx context.Context, name string) (*User, error) {
 	if name == "" {
 		return nil, ErrEmptyName
 	}
-	row := s.db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE name = $1", name)
+	q := "SELECT id, name FROM users WHERE name = '" + name + "'"
+	row := s.db.QueryRowContext(ctx, q)
 	var u User
 	return &u, row.Scan(&u.ID, &u.Name)
`

func mustParse(t *testing.T, s string) *Command {
	t.Helper()
	c, err := ParseCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRunCommand_ExplainIsScopedToTheSelection(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"answer":"It concatenates **name** into SQL."}`}
	})
	thread := []ThreadMessage{{Author: "alice", Body: "Is this safe?"}, {Author: "bob", Body: "Not sure."}}
	res, err := newEngine(t, srv).RunCommand(context.Background(), strings.NewReader(twoHunks), mustParse(t, "/explain internal/store/users.go:45-46 is it injectable?"), thread)
	if err != nil {
		t.Fatal(err)
	}
	if res.Answer != "It concatenates **name** into SQL." {
		t.Errorf("answer %q", res.Answer)
	}
	req := srv.Requests()[0]
	sys, user := req.Messages[0].Content, llmtest.UserPrompt(&req)
	if !strings.Contains(sys, "answering a reviewer's /explain command") {
		t.Error("explain did not use the explain template")
	}
	for _, want := range []string{"/explain internal/store/users.go:45-46: is it injectable?", "@@ -41,6 +42,7 @@", "alice: Is this safe?\nbob: Not sure.\n"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "SetMaxOpenConns") || strings.Contains(user, "@@ -10,3") {
		t.Error("prompt includes a hunk outside the selection")
	}
}

func TestRunCommand_RefactorStaysInsideTheSelection(t *testing.T) {
	outside := `{"suggestions":[{"file":"internal/store/users.go","start_line":42,"end_line":48,"category":"Security","summary":"s","existing_code":"x","improved_code":"y"}]}`
	inside := `{"suggestions":[{"file":"internal/store/users.go","start_line":45,"end_line":46,"category":"Security","summary":"Bind the parameter","existing_code":"q := ...","improved_code":"row := s.db.QueryRowContext(ctx, query, name)"}]}`
	srv := llmtest.NewServer(t, func(n int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		if n == 1 {
			return llmtest.Reply{Content: `{"suggestions":[{"file":"internal/store/users.go","start_line":11,"end_line":11,"category":"Logic","summary":"s","existing_code":"x","improved_code":"y"}]}`}
		}
		if n == 2 {
			return llmtest.Reply{Content: outside}
		}
		return llmtest.Reply{Content: inside}
	})
	res, err := newEngine(t, srv, func(c *Config) { c.MaxRepairs = 3 }).RunCommand(context.Background(), strings.NewReader(twoHunks), mustParse(t, "/refactor internal/store/users.go:45-46"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Suggestions) != 1 || res.Suggestions[0].StartLine != 45 {
		t.Errorf("suggestions %+v", res.Suggestions)
	}
	reqs := srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("%d requests, want 3 (two repairs)", len(reqs))
	}
	// Line 11 is in the diff but in another hunk: rejected by the hunk anchor.
	// Lines 42-48 overlap the selection but reach outside it: rejected too.
	if msg := reqs[1].Messages[len(reqs[1].Messages)-1].Content; !strings.Contains(msg, "lines 11-11 of internal/store/users.go are outside the changed hunks") {
		t.Errorf("first repair:\n%s", msg)
	}
	if msg := reqs[2].Messages[len(reqs[2].Messages)-1].Content; !strings.Contains(msg, "lines 42-48 reach outside the selection internal/store/users.go:45-46; refactor only lines 45-46") {
		t.Errorf("second repair:\n%s", msg)
	}
	if !strings.Contains(reqs[0].Messages[0].Content, "/refactor command") {
		t.Error("refactor did not use the refactor template")
	}
}

func TestRunCommand_Docstrings(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"suggestions":[{"file":"internal/store/users.go","start_line":11,"end_line":11,"category":"Maintainability","summary":"Document the pool size","existing_code":"\tdb.SetMaxOpenConns(10)","improved_code":"\t// Cap the pool.\n\tdb.SetMaxOpenConns(10)"}]}`}
	})
	res, err := newEngine(t, srv).RunCommand(context.Background(), strings.NewReader(twoHunks), mustParse(t, "/docstrings internal/store/users.go"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Suggestions) != 1 {
		t.Fatalf("suggestions %+v", res.Suggestions)
	}
	user := llmtest.UserPrompt(&srv.Requests()[0])
	if !strings.Contains(user, "@@ -10,3") || !strings.Contains(user, "@@ -41,6") {
		t.Error("/docstrings should see every hunk of the file")
	}
}

func TestRunCommand_ScopeErrorsMakeNoRequest(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"answer":"x"}`}
	})
	e := newEngine(t, srv)
	for in, want := range map[string]string{
		"/explain internal/api/other.go:3":       "internal/api/other.go is not part of this diff",
		"/explain internal/store/users.go:20-30": "internal/store/users.go:20-30 has no changed lines in this diff",
	} {
		_, err := e.RunCommand(context.Background(), strings.NewReader(twoHunks), mustParse(t, in), nil)
		if !errors.Is(err, ErrBadCommand) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("%d requests for out-of-scope commands", len(srv.Requests()))
	}
}

func TestRunCommand_ExplainValidated(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply { return llmtest.Reply{Content: `{"answer":""}`} })
	_, err := newEngine(t, srv, func(c *Config) { c.MaxRepairs = -1 }).RunCommand(context.Background(), strings.NewReader(twoHunks), mustParse(t, "/explain internal/store/users.go:11"), nil)
	if !errors.Is(err, schema.ErrInvalid) {
		t.Errorf("empty answer: err = %v, want schema.ErrInvalid", err)
	}
}

func TestRecentThread_KeepsNewestWithinBudget(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	e := newEngine(t, srv)
	long := strings.Repeat("word ", 600)
	thread := []ThreadMessage{{Author: "a", Body: long}, {Author: "b", Body: long}, {Author: "c", Body: long}, {Author: "d", Body: "latest"}}
	got := e.recentThread(thread)
	if len(got) == 0 || got[len(got)-1].Body != "latest" || got[0].Author == "a" {
		t.Errorf("kept %d messages starting with %q; want the newest within %d tokens", len(got), got[0].Author, maxThreadTokens)
	}
	if n := len(e.recentThread(nil)); n != 0 {
		t.Errorf("nil thread gave %d messages", n)
	}
}

func TestFormatCommandResult(t *testing.T) {
	explain := FormatCommandResult(&CommandResult{Command: Command{Kind: CommandExplain, File: "a.go", StartLine: 3, EndLine: 4, Question: "why?"}, Answer: "Because.\n"})
	if explain != "**/explain** `a.go:3-4`: why?\n\nBecause.\n" {
		t.Errorf("explain:\n%q", explain)
	}
	refactor := FormatCommandResult(&CommandResult{Command: Command{Kind: CommandRefactor, File: "a.go", StartLine: 3, EndLine: 3},
		Suggestions: []CodeSuggestion{{File: "a.go", StartLine: 3, EndLine: 3, Summary: "Simplify", ImprovedCode: "x := 1"}}})
	if !strings.Contains(refactor, "Simplify · `a.go:3`\n\n```go\nx := 1\n```") {
		t.Errorf("refactor:\n%s", refactor)
	}
	none := FormatCommandResult(&CommandResult{Command: Command{Kind: CommandDocstrings, File: "a.go"}, Suggestions: []CodeSuggestion{}})
	if !strings.Contains(none, "No changes suggested.") {
		t.Errorf("empty:\n%s", none)
	}
}

// /reconsider (ISSUE-029): the model re-evaluates the disputed finding,
// shown to it with the thread, and its verdict is schema-validated.
func TestRunCommand_Reconsider(t *testing.T) {
	srv := llmtest.NewServer(t, func(n int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		if n == 1 {
			return llmtest.Reply{Content: `{"verdict":"maybe","explanation":"x"}`} // not a verdict: repaired
		}
		return llmtest.Reply{Content: `{"verdict":"withdrawn","explanation":"The name is bound as a parameter on line 46."}`}
	})
	cmd := mustParse(t, "/reconsider internal/store/users.go:45-46 it is bound upstream")
	cmd.Finding = "Major (Security): SQL injection\n\nThe name is concatenated."
	thread := []ThreadMessage{{Author: "vizzini-bot", Body: "finding"}, {Author: "dev", Body: "It is bound upstream."}}
	res, err := newEngine(t, srv).RunCommand(context.Background(), strings.NewReader(twoHunks), cmd, thread)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictWithdrawn || !strings.Contains(res.Answer, "bound as a parameter") {
		t.Errorf("result %+v", res)
	}
	req := srv.Requests()[0]
	sys, user := req.Messages[0].Content, llmtest.UserPrompt(&req)
	if !strings.Contains(sys, "a\ndeveloper disputes it") && !strings.Contains(sys, "developer disputes it") {
		t.Error("reconsider did not use the reconsider template")
	}
	for _, want := range []string{"/reconsider internal/store/users.go:45-46: it is bound upstream", "<finding>\nMajor (Security): SQL injection",
		"dev: It is bound upstream."} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q:\n%s", want, user)
		}
	}
	md := FormatCommandResult(res)
	if !strings.Contains(md, "**Withdrawn.**") || !strings.Contains(md, "bound as a parameter") {
		t.Errorf("markdown:\n%s", md)
	}
	res.Verdict = VerdictStands
	if md := FormatCommandResult(res); !strings.Contains(md, "**The finding stands.**") || !strings.Contains(md, "a person decides") {
		t.Errorf("stands markdown:\n%s", md)
	}

	// Without the finding there is nothing to reconsider, and no request.
	n := len(srv.Requests())
	_, err = newEngine(t, srv).RunCommand(context.Background(), strings.NewReader(twoHunks), mustParse(t, "/reconsider internal/store/users.go:45"), nil)
	if !errors.Is(err, ErrBadCommand) || len(srv.Requests()) != n {
		t.Errorf("no finding: %v, %d requests", err, len(srv.Requests())-n)
	}
}
