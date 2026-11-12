package main

import (
	vmarker "github.com/infrashift/vizzini-pr-reviewer/pkg/marker"

	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

func intp(n int) *int { return &n }

func TestPendingCommands(t *testing.T) {
	threads := []remoteThread{
		{ID: "1", Path: "a.go", Line: intp(3), Comments: []remoteComment{
			{ID: "10", Author: "bot", Body: "finding <!-- vizzini:fp=v1:x run=r head=abcdef0 lines=3-3 cat=Logic -->"},
			{ID: "11", Author: "ana", Body: "Why?\n/vizzini explain is this safe?"},
			{ID: "12", Author: "bot", Body: "answer <!-- vizzini:reply-to=11 run=r -->"},
			{ID: "13", Author: "ben", Body: "/VIZZINI refactor make it a loop"},
		}},
		{ID: "2", Path: "b.go", Comments: []remoteComment{
			{ID: "20", Author: "ana", Body: "please /vizzini explain"}, // not on a line of its own
			{ID: "21", Author: "mallory", Body: "/vizzini docstrings"},
		}},
	}
	got := pendingCommands(threads, nil)
	ids := make([]string, 0, len(got))
	for _, p := range got {
		ids = append(ids, p.Comment.ID)
	}
	if !slices.Equal(ids, []string{"13", "21"}) {
		t.Fatalf("pending %v, want 13 and 21 (11 is answered, 20 is prose)", ids)
	}
	if got[0].Text != "refactor make it a loop" || len(got[0].History) != 3 || got[0].History[1].Author != "ana" {
		t.Errorf("command 13: %+v", got[0])
	}
	// --command-authors limits who may run commands.
	if only := pendingCommands(threads, []string{"ben"}); len(only) != 1 || only[0].Comment.ID != "13" {
		t.Errorf("authors filter: %+v", only)
	}
}

func TestThreadCommand(t *testing.T) {
	ranged := &remoteThread{Path: "src/a.go", Line: intp(12), StartLine: intp(10)}
	single := &remoteThread{Path: "src/a.go", Line: intp(7)}
	loose := &remoteThread{Path: "src/a.go"}
	for _, tc := range []struct {
		text   string
		thread *remoteThread
		want   reviewer.Command
	}{
		{"explain is this safe?", ranged, reviewer.Command{Kind: "explain", File: "src/a.go", StartLine: 10, EndLine: 12, Question: "is this safe?"}},
		{"refactor", single, reviewer.Command{Kind: "refactor", File: "src/a.go", StartLine: 7, EndLine: 7}},
		{"explain src/b.go:3-4 and this?", single, reviewer.Command{Kind: "explain", File: "src/b.go", StartLine: 3, EndLine: 4, Question: "and this?"}},
		{"docstrings", loose, reviewer.Command{Kind: "docstrings", File: "src/a.go"}},
		{"docstrings src/a.go be brief", loose, reviewer.Command{Kind: "docstrings", File: "src/a.go", Question: "be brief"}},
	} {
		got, err := threadCommand(tc.text, tc.thread)
		if err != nil || *got != tc.want {
			t.Errorf("%q: %+v %v, want %+v", tc.text, got, err, tc.want)
		}
	}
	for _, bad := range []string{"explain", "dance now"} {
		if _, err := threadCommand(bad, loose); !errors.Is(err, reviewer.ErrBadCommand) {
			t.Errorf("%q: %v, want a bad command", bad, err)
		}
	}
}

// End to end: a run answers a pending command once, by mrman review reply,
// and a malformed one with its usage; the next run answers nothing again.
func TestBot_AnswersCommandsOnce(t *testing.T) {
	repo := gitRepo(t)
	srv := llmtest.NewServer(t, func(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
		if strings.Contains(llmtest.UserPrompt(req), "Is this safe?") {
			return llmtest.Reply{Content: `{"answer":"No: the name is concatenated into the SQL."}`}
		}
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[]}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	threads := func(extra ...map[string]any) string {
		comments := make([]any, 0, 3+len(extra))
		comments = append(comments,
			map[string]any{"id": "1", "author": "bot", "body": "finding\n\n" +
				(&marker{FP: "v1:x", Run: "r0", Head: strings.Repeat("ab", 20), Category: "Security", Start: 3, End: 3}).String()},
			map[string]any{"id": "2", "author": "ana", "body": "/vizzini explain Is this safe?"},
			map[string]any{"id": "3", "author": "ben", "body": "/vizzini dance"},
		)
		for _, e := range extra {
			comments = append(comments, e)
		}
		b, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
			"id": "77", "path": "store.go", "line": 3, "comments": comments}}})
		return string(b)
	}
	f := &fakeMrman{threadsOut: threads()}
	f.install(t)

	code, stdout, stderr := runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p", "--pr", "ado:o/p/r#7")
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil || !slices.Equal(res.Answered, []string{"2", "3"}) {
		t.Fatalf("exit %d, result %s: %s", code, stdout, stderr)
	}
	var replies [][]string
	for _, c := range f.calls {
		if len(c) > 1 && c[1] == "reply" {
			replies = append(replies, c)
		}
	}
	if len(replies) != 2 {
		t.Fatalf("replies %v", replies)
	}
	read := func(call []string) string {
		for i, a := range call {
			if a == "--input" {
				// The temp file is gone after the call; the fake recorded its body.
				return f.inputs[strings.TrimPrefix(call[i+1], "@")]
			}
		}
		return ""
	}
	first, second := read(replies[0]), read(replies[1])
	if !strings.Contains(strings.Join(replies[0], " "), "--thread 77 --parent 2") ||
		!strings.Contains(first, "concatenated into the SQL") || !strings.Contains(first, "<!-- vizzini:reply-to=2 run=") || !strings.Contains(first, " head=") {
		t.Errorf("reply to 2: %v\n%s", replies[0], first)
	}
	if !strings.Contains(second, "I could not run that") || !strings.Contains(second, "<!-- vizzini:reply-to=3 run=") {
		t.Errorf("reply to 3 (usage):\n%s", second)
	}

	// The command's model call is attributed like the review's (ADR-0015).
	var commandMeta string
	for i, req := range srv.Requests() {
		if strings.Contains(llmtest.UserPrompt(&req), "Is this safe?") {
			commandMeta = srv.Headers()[i].Get("x-litellm-spend-logs-metadata")
		}
	}
	for _, want := range []string{`"vizzini.mode":"command"`, `"vizzini.run_id":"` + res.RunID + `"`, `"vizzini.agent":"vizzini-bot"`,
		`"vizzini.pr_key":"ado:o/p/r#7"`, `"vizzini.iteration":"` + res.Head + `"`} {
		if !strings.Contains(commandMeta, want) {
			t.Errorf("command request metadata %s lacks %s", commandMeta, want)
		}
	}

	// Next run: both carry a reply, so nothing is answered again.
	f = &fakeMrman{threadsOut: threads(
		map[string]any{"id": "4", "author": "bot", "body": first},
		map[string]any{"id": "5", "author": "bot", "body": second})}
	f.install(t)
	_, stdout, _ = runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p")
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || len(res.Answered) != 0 {
		t.Errorf("second run answered again: %s", stdout)
	}

	// A denied reply exits 3.
	f = &fakeMrman{threadsOut: threads(), replyOut: `{"error":"policy_denied"}`, replyErr: errExit1}
	f.install(t)
	if code, _, _ := runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p"); code != exitDenied {
		t.Errorf("denied reply: exit %d", code)
	}

	// --max-commands 0 answers none.
	f = &fakeMrman{threadsOut: threads()}
	f.install(t)
	_, stdout, _ = runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p", "--max-commands", "0")
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || len(res.Answered) != 0 {
		t.Errorf("--max-commands 0: %s", stdout)
	}
}

// --commands-only (vizzini-webhook's mode) answers the pending commands and
// does nothing else: no review request, no submit, no resolution.
func TestBot_CommandsOnly(t *testing.T) {
	repo := gitRepo(t)
	srv := llmtest.NewServer(t, func(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
		if strings.Contains(llmtest.UserPrompt(req), "Is this safe?") {
			return llmtest.Reply{Content: `{"answer":"No."}`}
		}
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[` +
			`{"file":"store.go","start_line":3,"end_line":3,"severity":"Critical","category":"Security","title":"t","body":"b"}]}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	// The bot's thread is from an older head, so a normal run would review.
	threads, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
		"id": "77", "path": "store.go", "line": 3, "comments": []any{
			map[string]any{"id": "1", "author": "bot", "body": "finding\n\n" +
				(&marker{FP: "v1:x", Run: "r0", Head: strings.Repeat("ab", 20), Category: "Security", Start: 3, End: 3}).String()},
			map[string]any{"id": "2", "author": "ana", "body": "/vizzini explain Is this safe?"},
		}}}})
	f := &fakeMrman{threadsOut: string(threads)}
	f.install(t)

	code, stdout, stderr := runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p", "--commands-only")
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil {
		t.Fatalf("exit %d, result %s: %s", code, stdout, stderr)
	}
	if res.Mode != modeCommands || !slices.Equal(res.Answered, []string{"2"}) || res.Findings != 0 || len(res.ReviewedFiles) != 0 {
		t.Errorf("result %s", stdout)
	}
	for _, c := range f.calls {
		if len(c) > 1 && (c[1] == "submit" || c[1] == "resolve" || c[1] == "add") {
			t.Errorf("--commands-only called mrman %v", c)
		}
	}
	for _, req := range srv.Requests() {
		if !strings.Contains(llmtest.UserPrompt(&req), "Is this safe?") {
			t.Errorf("--commands-only sent a non-command model request")
		}
	}

	// It refuses combinations that would make it do nothing or post nothing.
	for _, args := range [][]string{{"--dry-run"}, {"--max-commands", "0"}} {
		a := append([]string{"--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p", "--commands-only"}, args...)
		if code, _, stderr := runBot(t, env, a...); code != exitError || !strings.Contains(stderr, "--commands-only") {
			t.Errorf("%v: exit %d, %s", args, code, stderr)
		}
	}
}

// /vizzini reconsider (ISSUE-029): a withdrawn finding resolves the bot's
// thread with the explanation; a finding that stands gets a reply and its
// thread stays open; outside a finding's thread it gets usage.
func TestBot_Reconsider(t *testing.T) {
	repo := gitRepo(t)
	verdict := "withdrawn"
	srv := llmtest.NewServer(t, func(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
		if strings.Contains(llmtest.UserPrompt(req), "/reconsider") {
			return llmtest.Reply{Content: `{"verdict":"` + verdict + `","explanation":"Checked against line 3."}`}
		}
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[]}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	finding := (&vmarker.Comment{Severity: "Major", Category: "Security", Title: "SQL injection", Body: "Concatenated.", Agent: "vizzini-bot",
		Marker: marker{FP: "v1:x", Run: "r0", Head: strings.Repeat("ab", 20), Category: "Security", Start: 3, End: 3}}).Render()
	threads := func(first string) string {
		b, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{"id": "77", "path": "store.go", "line": 3, "comments": []any{
			map[string]any{"id": "1", "author": "bot", "body": first},
			map[string]any{"id": "2", "author": "dev", "body": "/vizzini reconsider The input is validated upstream."},
		}}}})
		return string(b)
	}
	args := []string{"--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p", "--commands-only"}
	calls := func(f *fakeMrman, verb string) [][]string {
		var out [][]string
		for _, c := range f.calls {
			if len(c) > 1 && c[1] == verb {
				out = append(out, c)
			}
		}
		return out
	}

	f := &fakeMrman{threadsOut: threads(finding), resolveOut: `{"resolved":["77"]}`}
	f.install(t)
	code, stdout, stderr := runBot(t, env, args...)
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil {
		t.Fatalf("exit %d: %s %s", code, stdout, stderr)
	}
	resolves := calls(f, "resolve")
	if len(resolves) != 1 || len(calls(f, "reply")) != 0 || !slices.Equal(res.Withdrawn, []string{"77"}) || !slices.Equal(res.Answered, []string{"2"}) {
		t.Fatalf("withdrawn: resolves %v, result %s", resolves, stdout)
	}
	joined := strings.Join(resolves[0], " ")
	for _, want := range []string{"--thread 77", "**Withdrawn.**", "Checked against line 3.", "<!-- vizzini:reply-to=2 run="} {
		if !strings.Contains(joined, want) {
			t.Errorf("resolve call lacks %q: %s", want, joined)
		}
	}
	for _, req := range srv.Requests() {
		if u := llmtest.UserPrompt(&req); strings.Contains(u, "/reconsider") && !strings.Contains(u, "<finding>\nMajor (Security): SQL injection\n\nConcatenated.") {
			t.Errorf("the finding was not given to the model:\n%s", u)
		}
	}

	verdict = "stands"
	f = &fakeMrman{threadsOut: threads(finding)}
	f.install(t)
	_, stdout, _ = runBot(t, env, args...)
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || len(calls(f, "resolve")) != 0 || len(calls(f, "reply")) != 1 || len(res.Withdrawn) != 0 {
		t.Errorf("stands: %s", stdout)
	}

	// Not a finding's thread: one usage reply, no model call for it.
	n := len(srv.Requests())
	f = &fakeMrman{threadsOut: threads("a person's question")}
	f.install(t)
	_, stdout, _ = runBot(t, env, args...)
	replies := calls(f, "reply")
	if len(replies) != 1 || len(srv.Requests()) != n {
		t.Fatalf("not a finding: replies %v, %d model calls: %s", replies, len(srv.Requests())-n, stdout)
	}
	for i, a := range replies[0] {
		if a == "--input" {
			if body := f.inputs[strings.TrimPrefix(replies[0][i+1], "@")]; !strings.Contains(body, "only in a thread vizzini-bot opened") {
				t.Errorf("usage reply: %s", body)
			}
		}
	}
}
