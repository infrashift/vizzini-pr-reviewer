package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

func TestMain(m *testing.M) { llmtest.RunOffline(m) }

// gitRepo makes a repository whose "feature" branch changes one line of
// store.go (line 3) relative to "main".
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // G204: fixed git subcommands from this test
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "store.go"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("package store\n\nconst q = \"SELECT * FROM users WHERE name = $1\"\n")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	git("checkout", "-q", "-b", "feature")
	write("package store\n\nconst q = \"SELECT * FROM users WHERE name = '\" + name + \"'\"\n")
	git("commit", "-q", "-am", "change")
	return dir
}

func mockProxy(t *testing.T) *llmtest.Server {
	t.Helper()
	return llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"Injection.","risk_score":90,"findings":[` +
			`{"file":"store.go","start_line":3,"end_line":3,"severity":"Critical","category":"Security","title":"SQL injection","body":"Concatenated input.","suggestion":"Bind it."}]}`}
	})
}

// fakeMrman intercepts mrman calls (git still runs) and records them.
type fakeMrman struct {
	calls      [][]string
	threadsOut string // `review threads` answer; default no threads
	submitOut  string
	submitErr  error
	resolveOut string
	resolveErr error
	replyOut   string
	replyErr   error
	// inputs holds each --input @file's content, read when mrman is called.
	inputs map[string]string
	// policy is agent-submit.cue as handed to the last submit.
	policy string
}

func (f *fakeMrman) install(t *testing.T) {
	t.Helper()
	realCommand := command
	command = func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
		if name != "mrman" {
			return realCommand(ctx, dir, name, args...)
		}
		f.calls = append(f.calls, args)
		switch {
		case args[0] == "pr":
			return []byte(`{"slug":"ado-org-proj-repo-7"}`), nil, nil
		case args[1] == "threads":
			return []byte(cmp.Or(f.threadsOut, `{"threads":[]}`)), nil, nil
		case args[1] == "submit":
			// The policy as mrman would read it, captured while it exists.
			if i := slices.Index(args, "--grant-policy"); i >= 0 && i+1 < len(args) {
				if data, err := os.ReadFile(filepath.Join(args[i+1], "agent-submit.cue")); err == nil { //nolint:gosec // G304: the bot's own temp copy
					f.policy = string(data)
				}
			}
			return []byte(f.submitOut), []byte("mrman: authorization: {}\n"), f.submitErr
		case args[1] == "resolve":
			return []byte(f.resolveOut), []byte("mrman: authorization: {}\n"), f.resolveErr
		case args[1] == "reply":
			for i, a := range args {
				if a == "--input" && i+1 < len(args) {
					file := strings.TrimPrefix(args[i+1], "@")
					data, _ := os.ReadFile(file) //nolint:gosec // G304: the bot's own temp file
					if f.inputs == nil {
						f.inputs = map[string]string{}
					}
					f.inputs[file] = string(data)
				}
			}
			return []byte(cmp.Or(f.replyOut, `{"thread":"77","comment_id":"99"}`)), nil, f.replyErr
		}
		return nil, nil, nil
	}
	t.Cleanup(func() { command = realCommand })
}

func runBot(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(context.Background(), args, &out, &errb, func(k string) string { return env[k] })
	return code, out.String(), errb.String()
}

func botEnv(t *testing.T) map[string]string {
	return map[string]string{"VIZZINI_LITELLM_URL": mockProxy(t).BaseURL(), "VIZZINI_MODEL": "m", "VIZZINI_API_KEY": "k"}
}

func TestBot_PostsThroughMrmanUnderTheGrantPolicy(t *testing.T) {
	repo := gitRepo(t)
	// The base branch carries the policy; the PR rewrites it to allow
	// itself more. mrman must be handed the base's.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // G204: fixed git subcommands from this test
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	policyFile := filepath.Join(repo, "policies", "mrman", "agent-submit.cue")
	writePolicy := func(body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(policyFile), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(policyFile, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("checkout", "-q", "main")
	writePolicy("package mrman\n_allowedEvents: [\"comment\"]\n")
	git("add", "policies")
	git("commit", "-q", "-m", "policy")
	git("checkout", "-q", "feature")
	git("merge", "-q", "--no-edit", "main")
	writePolicy("package mrman\n_allowedEvents: [\"comment\", \"approve\"]\n")
	git("commit", "-q", "-am", "let me approve myself")

	f := &fakeMrman{submitOut: `{"state":"COMMENTED"}`}
	f.install(t)
	out := filepath.Join(t.TempDir(), "review.json")

	code, _, stderr := runBot(t, botEnv(t), "--repo", repo, "--base", "main", "--pr-url", "https://dev.azure.com/o/p/_git/r/pullrequest/7",
		"--grant-policy", "policies/mrman", "--run-id", "run-1", "--out", out)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(f.calls) != 4 || f.calls[1][1] != "threads" {
		t.Fatalf("mrman calls %v", f.calls)
	}
	add := f.calls[2]
	if !slices.Equal(add[:9], []string{"review", "add", "--session", "ado-org-proj-repo-7", "--target-file", "store.go", "--line", "3", "--side"}) ||
		!strings.Contains(add[len(add)-1], "<!-- vizzini:fp=v1:") || !strings.Contains(add[len(add)-1], " run=run-1 head=") || !strings.Contains(add[len(add)-1], " lines=3-3 cat=Security -->") {
		t.Errorf("review add %q", add)
	}
	submit := strings.Join(f.calls[3], " ")
	for _, want := range []string{"--event comment", "--grant-policy ", "--run-id run-1", "--agent vizzini-bot", "--garmr garmr"} {
		if !strings.Contains(submit, want) {
			t.Errorf("submit %q lacks %q", submit, want)
		}
	}
	if !strings.Contains(f.policy, `_allowedEvents: ["comment"]`) || strings.Contains(f.policy, "approve") {
		t.Errorf("mrman evaluated %q, want the base branch's policy", f.policy)
	}
	if !strings.Contains(stderr, "mrman: authorization:") {
		t.Errorf("the authorization line must reach the CI log: %s", stderr)
	}

	var report struct {
		Kind      string `json:"kind"`
		RunID     string `json:"run_id"`
		Agent     string `json:"agent"`
		Iteration string `json:"iteration"`
	}
	data, err := os.ReadFile(out) //nolint:gosec // G304: the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &report); err != nil || report.Kind != "Review" || report.RunID != "run-1" ||
		report.Agent != "vizzini-bot" || len(report.Iteration) != 40 {
		t.Errorf("report %+v (%v)", report, err)
	}
}

func TestBot_PolicyDenialPostsNothingAndExits3(t *testing.T) {
	repo := gitRepo(t)
	f := &fakeMrman{submitOut: `{"error":"policy_denied","message":"denied"}`, submitErr: errors.New("exit status 1")}
	f.install(t)
	code, stdout, stderr := runBot(t, botEnv(t), "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p")
	if code != exitDenied || !strings.Contains(stderr, "denied posting") || !strings.Contains(stdout, "policy_denied") {
		t.Errorf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
}

// A transient forge error is retried; a persistent one fails the run.
func TestBot_RetriesForgeNetworkErrors(t *testing.T) {
	repo := gitRepo(t)
	retryDelay = func(int) time.Duration { return 0 }
	t.Cleanup(func() { retryDelay = func(n int) time.Duration { return time.Duration(n) * 5 * time.Second } })
	for _, tc := range []struct {
		failures, wantCode, wantPr int
	}{
		{1, exitOK, 2},    // fails once, then opens
		{5, exitError, 3}, // gives up after three attempts
	} {
		f := &fakeMrman{submitOut: `{}`}
		f.install(t)
		inner, prCalls := command, 0
		command = func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			if name == "mrman" && args[0] == "pr" {
				prCalls++
				if prCalls <= tc.failures {
					return nil, []byte("error: azure_devops: get_pull_request: network error on dev.azure.com: TLS handshake timeout"), errors.New("exit status 1")
				}
			}
			return inner(ctx, dir, name, args...)
		}
		code, _, stderr := runBot(t, botEnv(t), "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p")
		if code != tc.wantCode || prCalls != tc.wantPr || !strings.Contains(stderr, "retrying") {
			t.Errorf("%d failures: exit %d after %d opens, want %d after %d: %s", tc.failures, code, prCalls, tc.wantCode, tc.wantPr, stderr)
		}
		command = inner
	}
}

func TestBot_SubmitFailureIsAnError(t *testing.T) {
	repo := gitRepo(t)
	f := &fakeMrman{submitOut: `{"error":"forge"}`, submitErr: errors.New("exit status 1")}
	f.install(t)
	if code, _, _ := runBot(t, botEnv(t), "--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "/p"); code != exitError {
		t.Errorf("exit %d, want %d", code, exitError)
	}
}

func TestBot_DryRunTouchesNoForge(t *testing.T) {
	repo := gitRepo(t)
	f := &fakeMrman{}
	f.install(t)
	code, stdout, stderr := runBot(t, botEnv(t), "--repo", repo, "--base", "main", "--dry-run")
	if code != exitOK || len(f.calls) != 0 {
		t.Fatalf("exit %d, mrman calls %v: %s", code, f.calls, stderr)
	}
	var c comment
	if err := json.Unmarshal([]byte(stdout), &c); err != nil || c.File != "store.go" || c.StartLine != 3 ||
		!strings.Contains(c.Body, "**Critical (Security): SQL injection**") || !strings.Contains(c.Body, "Bind it.") {
		t.Errorf("planned comment %+v (%v)", c, err)
	}
}

// A lint report reaches the review prompt.
func TestBot_LintJSON(t *testing.T) {
	repo := gitRepo(t)
	srv := llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":10,"findings":[]}`}
	})
	lint := filepath.Join(t.TempDir(), "lint.json")
	report := `{"Issues":[{"FromLinter":"gosec","Text":"G202: SQL string concatenation","Pos":{"Filename":"store.go","Line":3,"Column":1}}]}`
	if err := os.WriteFile(lint, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	if code, _, stderr := runBot(t, env, "--repo", repo, "--base", "main", "--dry-run", "--lint-json", lint); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || !strings.Contains(llmtest.UserPrompt(&reqs[0]), "G202") {
		t.Error("the lint diagnostic did not reach the prompt")
	}
	if code, _, stderr := runBot(t, env, "--repo", repo, "--base", "main", "--dry-run", "--lint-json", "/nonexistent"); code != exitError ||
		!strings.Contains(stderr, "--lint-json") {
		t.Errorf("a missing report: exit %d: %s", code, stderr)
	}
}

func TestParseFlags_PostingNeedsAPolicy(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--pr-url", "u", "--grant-policy", "/p"}, "--base is required"},
		{[]string{"--base", "main", "--grant-policy", "/p"}, "--pr-url is required"},
		{[]string{"--base", "main", "--pr-url", "u"}, "--grant-policy is required"},
		{[]string{"--base", "main", "--dry-run", "extra"}, "unexpected arguments"},
	} {
		if _, err := parseFlags(tc.args, &bytes.Buffer{}, func(string) string { return "" }); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: %v, want %q", tc.args, err, tc.want)
		}
	}
	o, err := parseFlags([]string{"--base", "main", "--dry-run"}, &bytes.Buffer{}, func(string) string { return "" })
	if err != nil || len(o.attr.RunID) != 36 || o.attr.Agent != "vizzini-bot" || o.head != "HEAD" {
		t.Errorf("defaults %+v (%v)", o, err)
	}
}

func TestBot_GitFailureIsReported(t *testing.T) {
	code, _, stderr := runBot(t, botEnv(t), "--repo", t.TempDir(), "--base", "main", "--dry-run")
	if code != exitError || !strings.Contains(stderr, "--head HEAD") {
		t.Errorf("exit %d: %s", code, stderr)
	}
}
