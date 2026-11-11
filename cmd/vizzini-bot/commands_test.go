package main

import (
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

	code, stdout, stderr := runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "p")
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

	// Next run: both carry a reply, so nothing is answered again.
	f = &fakeMrman{threadsOut: threads(
		map[string]any{"id": "4", "author": "bot", "body": first},
		map[string]any{"id": "5", "author": "bot", "body": second})}
	f.install(t)
	_, stdout, _ = runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "p")
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || len(res.Answered) != 0 {
		t.Errorf("second run answered again: %s", stdout)
	}

	// A denied reply exits 3.
	f = &fakeMrman{threadsOut: threads(), replyOut: `{"error":"policy_denied"}`, replyErr: errExit1}
	f.install(t)
	if code, _, _ := runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "p"); code != exitDenied {
		t.Errorf("denied reply: exit %d", code)
	}

	// --max-commands 0 answers none.
	f = &fakeMrman{threadsOut: threads()}
	f.install(t)
	_, stdout, _ = runBot(t, env, "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "p", "--max-commands", "0")
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || len(res.Answered) != 0 {
		t.Errorf("--max-commands 0: %s", stdout)
	}
}
