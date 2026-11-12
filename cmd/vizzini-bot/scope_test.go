package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

// scopeRepo: main has a.go, b.go; feature changes both (commit "pr").
func scopeRepo(t *testing.T) (dir string, git func(args ...string) string, write func(name, body string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir = t.TempDir()
	git = func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // G204: fixed git subcommands from this test
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write = func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("a.go", "package x\n\nvar A = 1\n")
	write("b.go", "package x\n\nvar B = 1\n")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	git("checkout", "-q", "-b", "feature")
	write("a.go", "package x\n\nvar A = 2\n")
	write("b.go", "package x\n\nvar B = 2\n")
	git("commit", "-q", "-am", "pr")
	return dir, git, write
}

// botThreadOn is a thread the bot posted on file at head.
func botThreadOn(id, file, head string) string {
	m := marker{FP: "v1:" + id, Run: "r0", Head: head, Category: "Logic", Start: 3, End: 3}
	b, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
		"id": id, "path": "/" + file, "comments": []any{map[string]any{"body": "x\n\n" + m.String()}}}}})
	return string(b)
}

func TestReviewScope(t *testing.T) {
	dir, git, write := scopeRepo(t)
	srv := llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[]}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	args := []string{"--repo", dir, "--base", "main", "--pr-url", "u", "--grant-policy", "/p"}
	reviewed := func() string { // the diff part of the last prompt
		reqs := srv.Requests()
		return llmtest.UserPrompt(&reqs[len(reqs)-1])
	}
	first := git("rev-parse", "HEAD")

	// A push touching only a.go, plus a merge from main adding c.go: only
	// a.go is reviewed; the thread on b.go is carried.
	write("a.go", "package x\n\nvar A = 3\n")
	git("commit", "-q", "-am", "push")
	git("checkout", "-q", "main")
	write("c.go", "package x\n\nvar C = 1\n")
	git("add", "c.go")
	git("commit", "-q", "-m", "upstream")
	git("checkout", "-q", "feature")
	git("merge", "-q", "--no-edit", "main")

	f := &fakeMrman{threadsOut: botThreadOn("8", "b.go", first)}
	f.install(t)
	code, stdout, stderr := runBot(t, env, args...)
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil || res.Mode != "incremental" ||
		res.Since != first || !slices.Equal(res.ReviewedFiles, []string{"a.go"}) || !slices.Equal(res.Carried, []string{"8"}) ||
		len(res.KeptOpen)+len(res.Resolved) != 0 {
		t.Fatalf("exit %d, result %s: %s", code, stdout, stderr)
	}
	if p := reviewed(); !strings.Contains(p, "a.go") || strings.Contains(p, "b.go") || strings.Contains(p, "c.go") {
		t.Errorf("the incremental prompt must cover a.go only:\n%s", p)
	}

	// A merge from main alone changes nothing the PR changes: no review.
	head := git("rev-parse", "HEAD")
	git("checkout", "-q", "main")
	write("d.go", "package x\n\nvar D = 1\n")
	git("add", "d.go")
	git("commit", "-q", "-m", "upstream 2")
	git("checkout", "-q", "feature")
	git("merge", "-q", "--no-edit", "main")
	before := len(srv.Requests())
	f = &fakeMrman{threadsOut: botThreadOn("9", "a.go", head)}
	f.install(t)
	_, stdout, _ = runBot(t, env, args...)
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Mode != "unchanged" || len(srv.Requests()) != before {
		t.Errorf("merge-only push: result %s, %d requests", stdout, len(srv.Requests())-before)
	}

	// --full, or a marker head that is not an ancestor (a rebased branch),
	// reviews everything.
	for _, tc := range []struct {
		name, threads string
		extra         []string
	}{
		{"--full", botThreadOn("9", "a.go", head), []string{"--full"}},
		{"rebased", botThreadOn("9", "a.go", strings.Repeat("ab", 20)), nil},
		{"first run", "", nil},
	} {
		f = &fakeMrman{threadsOut: tc.threads}
		f.install(t)
		_, stdout, _ = runBot(t, env, append(slices.Clone(args), tc.extra...)...)
		var res botResult
		if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Mode != "full" || res.Since != "" {
			t.Errorf("%s: result %s", tc.name, stdout)
		}
		if p := reviewed(); !strings.Contains(p, "a.go") || !strings.Contains(p, "b.go") {
			t.Errorf("%s: a full review covers the whole PR", tc.name)
		}
	}
}

// Every bot comment records the head its run reviewed: a resolution or a
// command answer is enough for the next run to know nothing changed.
func TestReviewedHeads(t *testing.T) {
	h1, h2, h3 := strings.Repeat("a1", 20), strings.Repeat("b2", 20), strings.Repeat("c3", 20)
	threads := []remoteThread{{ID: "1", Comments: []remoteComment{
		{Body: "finding " + (&marker{FP: "v1:x", Run: "r", Head: h1, Category: "Logic", Start: 1, End: 1}).String()},
		{Body: "Resolved by vizzini-bot …\n\n<!-- vizzini:reviewed head=" + h2 + " run=r2 -->"},
		{Body: "an answer <!-- vizzini:reply-to=9 run=r3 head=" + h3 + " -->"},
		{Body: "a person writing head=" + strings.Repeat("d4", 20)},
		{Body: "slice 1 <!-- vizzini:fp=v1:y run=r0 -->"},
	}}}
	if got := reviewedHeads(threads); !slices.Equal(got, []string{h1, h2, h3}) {
		t.Errorf("heads %v", got)
	}
}

func TestReviewScope_AResolutionRecordsTheHead(t *testing.T) {
	dir, git, _ := scopeRepo(t)
	srv := llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[]}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	old := strings.Repeat("ab", 20) // not in this repository
	head := git("rev-parse", "HEAD")
	b, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
		"id": "5", "path": "a.go", "is_resolved": true, "comments": []any{
			map[string]any{"id": "1", "body": "finding\n\n" + (&marker{FP: "v1:z", Run: "r0", Head: old, Category: "Logic", Start: 3, End: 3}).String()},
			map[string]any{"id": "2", "body": "Resolved.\n\n<!-- vizzini:reviewed head=" + head + " run=r1 -->"},
		}}}})
	f := &fakeMrman{threadsOut: string(b)}
	f.install(t)
	_, stdout, stderr := runBot(t, env, "--repo", dir, "--base", "main", "--pr-url", "u", "--grant-policy", "/p")
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Mode != "unchanged" || len(srv.Requests()) != 0 {
		t.Errorf("result %s, %d requests: %s", stdout, len(srv.Requests()), stderr)
	}
}

// A file the push removes from the pull request (deleted, or reverted to the
// base) has nothing to review, but its thread is reconciled: its code
// changed and its finding is gone, so the thread is resolved, not carried.
func TestReviewScope_AFileThatLeftThePRIsReconciled(t *testing.T) {
	dir, git, write := scopeRepo(t)
	srv := llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[]}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	args := []string{"--repo", dir, "--base", "main", "--pr-url", "u", "--grant-policy", "/p"}
	first := git("rev-parse", "HEAD")
	threads := func(head string) string {
		all := make([]any, 0, 2)
		for _, f := range []string{"a.go", "b.go"} {
			var one struct{ Threads []any }
			_ = json.Unmarshal([]byte(botThreadOn(strings.TrimSuffix(f, ".go"), f, head)), &one)
			all = append(all, one.Threads...)
		}
		b, _ := json.Marshal(map[string]any{"threads": all})
		return string(b)
	}

	// b.go reverted to the base, a.go changed again: a.go is reviewed, and
	// b.go's thread is resolved.
	write("a.go", "package x\n\nvar A = 3\n")
	write("b.go", "package x\n\nvar B = 1\n")
	git("commit", "-q", "-am", "revert b, touch a")
	f := &fakeMrman{threadsOut: threads(first), resolveOut: `{"resolved":["b"]}`}
	f.install(t)
	code, stdout, stderr := runBot(t, env, args...)
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil || res.Mode != "incremental" ||
		!slices.Equal(res.ReviewedFiles, []string{"a.go"}) || !slices.Equal(res.Resolved, []string{"b"}) || len(res.Carried) != 0 {
		t.Fatalf("exit %d, result %s: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "1 file(s) with threads left the pull request") {
		t.Errorf("stderr: %s", stderr)
	}

	// Only the deletion of a file the PR itself added (not on the base):
	// nothing in the PR's diff changed, so no review at all, and the file's
	// thread is resolved.
	write("e.go", "package x\n\nvar E = 1\n")
	git("add", "e.go")
	git("commit", "-q", "-m", "add e")
	head := git("rev-parse", "HEAD")
	git("rm", "-q", "e.go")
	git("commit", "-q", "-m", "drop e")
	before := len(srv.Requests())
	f = &fakeMrman{threadsOut: botThreadOn("e", "e.go", head), resolveOut: `{"resolved":["e"]}`}
	f.install(t)
	_, stdout, stderr = runBot(t, env, args...)
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Mode != "incremental" || len(res.ReviewedFiles) != 0 ||
		!slices.Equal(res.Resolved, []string{"e"}) || len(srv.Requests()) != before {
		t.Errorf("deletion only: result %s, %d model requests: %s", stdout, len(srv.Requests())-before, stderr)
	}
}
