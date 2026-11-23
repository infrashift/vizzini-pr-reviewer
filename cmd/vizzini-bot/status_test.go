package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

func TestStatusBody(t *testing.T) {
	h := func(i int) string { return fmt.Sprintf("%040x", i) }
	first := statusBody("", h(1), "r1")
	if !strings.Contains(first, statusMarker) || !strings.Contains(first, "through `"+h(1)[:12]+"` (run `r1`)") {
		t.Errorf("first status %q", first)
	}
	if got := statusHeads([]remoteSummary{{ID: "9", Body: first}}); !slices.Equal(got, []string{h(1)}) {
		t.Errorf("heads %v", got)
	}

	// Each edit keeps the earlier heads, oldest first, and at most
	// maxStatusHeads of them.
	body := first
	for i := 2; i <= maxStatusHeads+5; i++ {
		body = statusBody(body, h(i), fmt.Sprintf("r%d", i))
	}
	got := statusHeads([]remoteSummary{{Body: body}})
	if len(got) != maxStatusHeads || got[0] != h(6) || got[len(got)-1] != h(maxStatusHeads+5) {
		t.Errorf("%d heads, first %s, last %s", len(got), got[0], got[len(got)-1])
	}
	// A head recorded again takes one line, moved to the end.
	body = statusBody(body, h(6), "r-again")
	got = statusHeads([]remoteSummary{{Body: body}})
	if len(got) != maxStatusHeads || got[0] != h(7) || got[len(got)-1] != h(6) || !strings.Contains(body, "head="+h(6)+" run=r-again -->") {
		t.Errorf("%d heads, first %s, last %s", len(got), got[0], got[len(got)-1])
	}
	if n := strings.Count(body, "<!-- vizzini:reviewed head="); n != maxStatusHeads {
		t.Errorf("%d head lines, want %d", n, maxStatusHeads)
	}

	// Only comments with the marker count; a person quoting a head does not.
	if got := statusHeads([]remoteSummary{{Body: "<!-- vizzini:reviewed head=" + h(1) + " run=x -->"}}); len(got) != 0 {
		t.Errorf("a comment without the status marker recorded %v", got)
	}
}

// quietEnv is a model that finds nothing.
func quietEnv(t *testing.T) (map[string]string, *llmtest.Server) {
	srv := llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":5,"findings":[]}`}
	})
	return map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}, srv
}

// threadsWith is a `review threads` answer with the given summaries.
func threadsWith(summaries ...remoteSummary) string {
	b, _ := json.Marshal(map[string]any{"threads": []any{}, "summaries": append([]remoteSummary{}, summaries...)})
	return string(b)
}

func botStatus(t *testing.T, f *fakeMrman, env map[string]string, dir string) (botResult, string) {
	t.Helper()
	return botStatusOn(t, f, env, dir, "u")
}

func botStatusOn(t *testing.T, f *fakeMrman, env map[string]string, dir, prURL string) (botResult, string) {
	t.Helper()
	f.install(t)
	code, stdout, stderr := runBot(t, env, "--repo", dir, "--base", "main", "--pr-url", prURL, "--grant-policy", "/p", "--run-id", "run-2")
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil {
		t.Fatalf("exit %d, result %s: %s", code, stdout, stderr)
	}
	return res, stderr
}

// calledWith returns the mrman calls whose subcommand is sub.
func calledWith(f *fakeMrman, sub string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if len(c) > 1 && c[1] == sub {
			out = append(out, c)
		}
	}
	return out
}

// A review that posts nothing records its head in a new status comment,
// a pull-request-level comment under the grant policy.
func TestBot_AQuietReviewPostsAStatusComment(t *testing.T) {
	dir, git, _ := scopeRepo(t)
	env, _ := quietEnv(t)
	f := &fakeMrman{threadsOut: threadsWith(remoteSummary{ID: "3", Author: "ana", Body: "LGTM"}), submitOut: `{"state":"COMMENTED"}`}
	res, stderr := botStatus(t, f, env, dir)
	head := git("rev-parse", "HEAD")

	adds := calledWith(f, "add")
	if res.Status != "posted" || res.Posted != 0 || len(adds) != 1 || len(calledWith(f, "edit")) != 0 {
		t.Fatalf("status %q, posted %d, adds %v: %s", res.Status, res.Posted, adds, stderr)
	}
	if slices.Contains(adds[0], "--target-file") || adds[0][3] != "ado-org-proj-repo-7" {
		t.Errorf("the status must be a pull-request-level comment: %v", adds[0])
	}
	body := adds[0][len(adds[0])-1]
	if !strings.Contains(body, statusMarker) || !strings.Contains(body, "<!-- vizzini:reviewed head="+head+" run=run-2 -->") {
		t.Errorf("status body %q", body)
	}
	submit := strings.Join(calledWith(f, "submit")[0], " ")
	for _, want := range []string{"--event comment", "--grant-policy ", "--run-id run-2", "--agent vizzini-bot"} {
		if !strings.Contains(submit, want) {
			t.Errorf("submit %q lacks %q", submit, want)
		}
	}
	// Not on Azure DevOps: nothing to resolve.
	if len(calledWith(f, "resolve")) != 0 {
		t.Errorf("resolved on a forge without comment state: %v", f.calls)
	}
}

// On Azure DevOps the status comment is a thread, created active: the bot
// resolves it so it cannot block the pull request.
func TestBot_TheStatusThreadIsResolvedOnAzureDevOps(t *testing.T) {
	dir, _, _ := scopeRepo(t)
	env, _ := quietEnv(t)
	f := &fakeMrman{threadsOut: threadsWith(), threadsLater: threadsWith(remoteSummary{ID: "632", Body: "x\n" + statusMarker}),
		submitOut: `{"state":"COMMENTED"}`, resolveOut: `{"resolved":["632"]}`}
	res, stderr := botStatusOn(t, f, env, dir, "https://dev.azure.com/o/p/_git/r/pullrequest/7")
	resolves := calledWith(f, "resolve")
	if res.Status != "posted" || len(resolves) != 1 {
		t.Fatalf("status %q, resolves %v: %s", res.Status, resolves, stderr)
	}
	r := strings.Join(resolves[0], " ")
	for _, want := range []string{"--thread 632", "--grant-policy ", "--run-id run-2", "--agent vizzini-bot"} {
		if !strings.Contains(r, want) {
			t.Errorf("resolve %q lacks %q", r, want)
		}
	}
	if strings.Contains(r, "--reply") || len(res.Resolved) != 0 {
		t.Errorf("the status thread is resolved silently and is not a finding's: %q, resolved %v", r, res.Resolved)
	}

	// A failed resolve is logged; the head is still recorded.
	f2 := &fakeMrman{threadsOut: threadsWith(), threadsLater: threadsWith(remoteSummary{ID: "632", Body: statusMarker}),
		resolveOut: `{"error":"policy_denied"}`, resolveErr: errors.New("exit status 1")}
	res, stderr = botStatusOn(t, f2, env, dir, "https://dev.azure.com/o/p/_git/r/pullrequest/7")
	if res.Status != "posted" || !strings.Contains(stderr, `the status comment is not resolved`) || !strings.Contains(stderr, `"resolve"`) {
		t.Errorf("status %q: %s", res.Status, stderr)
	}
}

func TestAzureDevOps(t *testing.T) {
	for u, want := range map[string]bool{
		"https://dev.azure.com/o/p/_git/r/pullrequest/7":      true,
		"https://org.visualstudio.com/p/_git/r/pullrequest/7": true,
		"https://github.com/o/r/pull/7":                       false,
		"https://gitlab.example.com/g/r/-/merge_requests/7":   false,
		"https://dev.azure.com.evil.example/x":                false,
		"u":                                                   false,
	} {
		if got := azureDevOps(u); got != want {
			t.Errorf("azureDevOps(%q) = %v", u, got)
		}
	}
}

// Later quiet reviews edit the status comment: one per pull request. Its
// heads count as reviewed, so the next run is incremental from them.
func TestBot_AQuietReviewEditsTheStatusComment(t *testing.T) {
	dir, git, write := scopeRepo(t)
	env, srv := quietEnv(t)
	first := git("rev-parse", "HEAD")
	write("b.go", "package x\n\nvar B = 3\n")
	git("commit", "-q", "-am", "push")
	head := git("rev-parse", "HEAD")

	f := &fakeMrman{threadsOut: threadsWith(remoteSummary{ID: "9", Author: "vizzini-bot", Body: statusBody("", first, "run-1")})}
	res, stderr := botStatus(t, f, env, dir)
	if res.Mode != scopeIncremental || res.Since != first || !slices.Equal(res.ReviewedFiles, []string{"b.go"}) {
		t.Errorf("mode %s since %s files %v: the status head must start the review", res.Mode, res.Since, res.ReviewedFiles)
	}
	edits := calledWith(f, "edit")
	if res.Status != "edited" || len(edits) != 1 || len(calledWith(f, "add")) != 0 || len(calledWith(f, "submit")) != 0 {
		t.Fatalf("status %q, edits %v, calls %v: %s", res.Status, edits, f.calls, stderr)
	}
	e := strings.Join(edits[0], " ")
	for _, want := range []string{"--summary 9", "--grant-policy ", "--run-id run-2", "--agent vizzini-bot",
		"head=" + first + " run=run-1 -->", "head=" + head + " run=run-2 -->"} {
		if !strings.Contains(e, want) {
			t.Errorf("edit %q lacks %q", e, want)
		}
	}
	if len(srv.Requests()) != 1 {
		t.Errorf("%d model requests, want 1", len(srv.Requests()))
	}

	// The next run finds this head in the status comment: nothing to review,
	// nothing to record.
	f2 := &fakeMrman{threadsOut: threadsWith(remoteSummary{ID: "9", Body: statusBody(statusBody("", first, "run-1"), head, "run-2")})}
	res, _ = botStatus(t, f2, env, dir)
	if res.Mode != scopeUnchanged || res.Status != "" || len(calledWith(f2, "edit"))+len(calledWith(f2, "add")) != 0 || len(srv.Requests()) != 1 {
		t.Errorf("mode %s status %q calls %v", res.Mode, res.Status, f2.calls)
	}
}

// A review that posts a finding records its head in the finding's marker;
// it writes no status comment.
func TestBot_AReviewThatPostsWritesNoStatus(t *testing.T) {
	repo := gitRepo(t)
	f := &fakeMrman{threadsOut: threadsWith(), submitOut: `{"state":"COMMENTED"}`}
	res, _ := botStatus(t, f, botEnv(t), repo)
	if res.Posted != 1 || res.Status != "" || len(calledWith(f, "add")) != 1 || len(calledWith(f, "edit")) != 0 {
		t.Errorf("posted %d, status %q, calls %v", res.Posted, res.Status, f.calls)
	}
}

// An mrman that lists no summaries (before review edit) cannot find the
// status comment, so the bot writes none rather than one per quiet run.
func TestBot_OldMrmanWritesNoStatus(t *testing.T) {
	dir, _, _ := scopeRepo(t)
	env, _ := quietEnv(t)
	f := &fakeMrman{threadsOut: `{"threads":[]}`}
	res, stderr := botStatus(t, f, env, dir)
	if res.Status != "" || len(calledWith(f, "add"))+len(calledWith(f, "edit"))+len(calledWith(f, "submit")) != 0 {
		t.Errorf("status %q, calls %v", res.Status, f.calls)
	}
	if !strings.Contains(stderr, "is not recorded") {
		t.Errorf("stderr must say the head is not recorded: %s", stderr)
	}
}

// A refused status write is reported, never fatal: the review stands.
func TestBot_AStatusWriteFailureIsNotFatal(t *testing.T) {
	dir, git, _ := scopeRepo(t)
	env, _ := quietEnv(t)
	first := git("rev-parse", "HEAD~1")
	f := &fakeMrman{threadsOut: threadsWith(remoteSummary{ID: "9", Body: statusBody("", first, "run-1")}),
		editOut: `{"error":"policy_denied","decision":"deny"}`, editErr: errors.New("exit status 1")}
	res, stderr := botStatus(t, f, env, dir)
	if res.Status != "failed" || !strings.Contains(stderr, `does not allow event "edit"`) {
		t.Errorf("status %q: %s", res.Status, stderr)
	}
}
