package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testSecret = "a-long-enough-test-secret"
	testRepo   = "dev.azure.com/org/proj/repo"
)

// sample is Azure DevOps' own sample "Pull request commented on" event
// (from its test-notification API), adjusted by edit.
func sample(t *testing.T, edit func(m map[string]any)) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/ado-pr-comment-event.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	field(pullRequest(m), "repository")["remoteUrl"] = "https://org@dev.azure.com/org/proj/_git/repo"
	comment(m)["content"] = "Thanks.\n/vizzini explain Is this safe?"
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	return b
}

// field walks m along keys, each an object.
func field(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		next, ok := m[k].(map[string]any)
		if !ok {
			panic("sample: no object at " + k)
		}
		m = next
	}
	return m
}

func comment(m map[string]any) map[string]any     { return field(m, "resource", "comment") }
func pullRequest(m map[string]any) map[string]any { return field(m, "resource", "pullRequest") }

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestNormalizeRepo(t *testing.T) {
	for in, want := range map[string]string{
		"https://org@dev.azure.com/org/proj/_git/repo":  testRepo,
		"https://dev.azure.com/org/proj/_git/repo":      testRepo,
		"dev.azure.com/org/proj/repo":                   testRepo,
		"DEV.AZURE.COM/org/proj/repo":                   testRepo,
		"https://dev.azure.com/org/My%20Project/_git/r": "dev.azure.com/org/My%20Project/r",
	} {
		if got, err := normalizeRepo(in); err != nil || got != want {
			t.Errorf("%s: %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{
		"https://fabrikam.visualstudio.com/DefaultCollection/_git/Fabrikam", // legacy host: not supported
		"https://github.com/o/r", "dev.azure.com/org/proj", "dev.azure.com/org/-x/repo", "dev.azure.com/org/proj/_git/a/b",
	} {
		if got, err := normalizeRepo(bad); err == nil {
			t.Errorf("%s: accepted as %q", bad, got)
		}
	}
}

func TestParseADOEvent(t *testing.T) {
	ev, err := parseADOEvent(sample(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	want := adoEvent{EventType: adoCommentEvent, CommentID: 2, CommentType: "text", Content: "Thanks.\n/vizzini explain Is this safe?",
		Author: "fabrikamfiber4@hotmail.com", PRID: 1, Status: "active", Repo: testRepo,
		CloneURL: "https://dev.azure.com/org/proj/_git/repo", Target: "master"}
	if *ev != want {
		t.Errorf("event\n got %+v\nwant %+v", *ev, want)
	}
	if ev.key() != "ado:dev.azure.com/org/proj/repo#1" || ev.job().WebURL != "https://dev.azure.com/org/proj/_git/repo/pullrequest/1" {
		t.Errorf("key %s, job %+v", ev.key(), ev.job())
	}

	// The unedited sample's legacy visualstudio.com host is refused.
	raw, _ := os.ReadFile("testdata/ado-pr-comment-event.json")
	if _, err := parseADOEvent(raw); err == nil || !strings.Contains(err.Error(), "dev.azure.com") {
		t.Errorf("legacy host: %v", err)
	}
	// Other events parse to their type only.
	if ev, err := parseADOEvent([]byte(`{"eventType":"git.push"}`)); err != nil || ev.EventType != "git.push" || ev.Repo != "" {
		t.Errorf("other event: %+v, %v", ev, err)
	}
	for name, edit := range map[string]func(map[string]any){
		"no PR id":           func(m map[string]any) { delete(pullRequest(m), "pullRequestId") },
		"option-like target": func(m map[string]any) { pullRequest(m)["targetRefName"] = "refs/heads/-x" },
		"dotted target":      func(m map[string]any) { pullRequest(m)["targetRefName"] = "refs/heads/a..b" },
		"tag target":         func(m map[string]any) { pullRequest(m)["targetRefName"] = "refs/tags/v1" },
	} {
		if _, err := parseADOEvent(sample(t, edit)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, bad := range []string{``, `[]`, `{}`, `not json`} {
		if _, err := parseADOEvent([]byte(bad)); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
}

func TestHasCommand(t *testing.T) {
	for in, want := range map[string]bool{
		"/vizzini explain":               true,
		"  /vizzini refactor x":          true,
		"ok\r\n/Vizzini docstrings a.go": true,
		"/vizzini":                       false,
		"see /vizzini explain":           false,
		"no command":                     false,
	} {
		if hasCommand(in) != want {
			t.Errorf("%q: want %v", in, want)
		}
	}
}

func testConfig(t *testing.T) *config {
	return &config{user: "vizzini", secret: testSecret, repos: map[string]bool{testRepo: true}, work: t.TempDir(),
		bot: "vizzini-bot", git: "git", grantPolicy: ".vizzini/grant-policy", maxCommands: 5, maxJobs: 1, jobTimeout: time.Minute}
}

func TestHandler(t *testing.T) {
	var mu sync.Mutex
	var ran []*job
	release := make(chan struct{})
	q := newQueue(1, func(_ context.Context, j *job) {
		<-release
		mu.Lock()
		ran = append(ran, j)
		mu.Unlock()
	}, testLog())
	s := &server{cfg: testConfig(t), queue: q, log: testLog()}
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	post := func(body []byte, user, pass string) (int, string, http.Header) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/hooks/azure-devops", bytes.NewReader(body))
		if user != "" || pass != "" {
			req.SetBasicAuth(user, pass)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header
	}
	good := sample(t, nil)

	if code, _, h := post(good, "", ""); code != http.StatusUnauthorized || !strings.Contains(h.Get("WWW-Authenticate"), "Basic") {
		t.Errorf("no auth: %d %v", code, h)
	}
	if code, _, _ := post(good, "vizzini", "wrong-but-long-password"); code != http.StatusUnauthorized {
		t.Errorf("wrong password: %d", code)
	}
	if code, _, _ := post(good, "other", testSecret); code != http.StatusUnauthorized {
		t.Errorf("wrong user: %d", code)
	}
	if code, _, _ := post([]byte("{"), "vizzini", testSecret); code != http.StatusBadRequest {
		t.Errorf("bad JSON: %d", code)
	}
	if code, _, _ := post(bytes.Repeat([]byte(" "), maxBody+10), "vizzini", testSecret); code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge body: %d", code)
	}
	for path, want := range map[string]int{"/hooks/azure-devops": http.StatusMethodNotAllowed, "/healthz": http.StatusOK} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+path, http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("GET %s: %d", path, resp.StatusCode)
		}
	}

	ignored := map[string][]byte{
		"not a pull request comment event": []byte(`{"eventType":"git.push"}`),
		"repository not allowlisted": sample(t, func(m map[string]any) {
			field(pullRequest(m), "repository")["remoteUrl"] = "https://dev.azure.com/org/proj/_git/other"
		}),
		"pull request not active": sample(t, func(m map[string]any) { pullRequest(m)["status"] = "completed" }),
		"not a text comment":      sample(t, func(m map[string]any) { comment(m)["commentType"] = "system" }),
		"written by the bot": sample(t, func(m map[string]any) {
			comment(m)["content"] = "/vizzini explain …\n<!-- vizzini:reply-to=2 run=r -->"
		}),
		"no /vizzini command": sample(t, func(m map[string]any) { comment(m)["content"] = "LGTM" }),
	}
	for reason, body := range ignored {
		code, out, _ := post(body, "vizzini", testSecret)
		if code != http.StatusOK || !strings.Contains(out, `"accepted":false`) || !strings.Contains(out, reason) {
			t.Errorf("%s: %d %s", reason, code, out)
		}
	}

	// Accepted: one job, and a second delivery while it waits is folded in.
	code, out, _ := post(good, "vizzini", testSecret)
	if code != http.StatusAccepted || !strings.Contains(out, `"queue":"queued"`) {
		t.Fatalf("accepted: %d %s", code, out)
	}
	close(release)
	q.close(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if len(ran) != 1 || ran[0].Key != "ado:dev.azure.com/org/proj/repo#1" || ran[0].Target != "master" || ran[0].PRID != 1 {
		t.Errorf("ran %+v", ran)
	}
	if code, out, _ := post(good, "vizzini", testSecret); code != http.StatusAccepted || !strings.Contains(out, stateClosed) {
		t.Errorf("after close: %d %s", code, out)
	}
}

func TestQueueCoalesces(t *testing.T) {
	var mu sync.Mutex
	runs := map[string]int{}
	gate := map[string]chan struct{}{"A": make(chan struct{}), "B": make(chan struct{})}
	started := make(chan string, 8)
	q := newQueue(1, func(_ context.Context, j *job) {
		started <- j.Key
		<-gate[j.Key]
		mu.Lock()
		runs[j.Key]++
		mu.Unlock()
	}, testLog())

	if s := q.add(&job{Key: "B"}); s != stateQueued {
		t.Fatal(s)
	}
	<-started // the only worker is busy with B
	if s := q.add(&job{Key: "A"}); s != stateQueued {
		t.Errorf("A: %s", s)
	}
	if s := q.add(&job{Key: "A"}); s != stateCoalesced {
		t.Errorf("A again while waiting: %s", s)
	}
	if s := q.add(&job{Key: "B"}); s != stateAgain {
		t.Errorf("B while running: %s", s)
	}
	gate["B"] <- struct{}{} // B's first run ends; A runs, then B once more
	if k := <-started; k != "A" {
		t.Errorf("next: %s", k)
	}
	close(gate["A"])
	if k := <-started; k != "B" {
		t.Errorf("then: %s", k)
	}
	close(gate["B"])
	q.close(context.Background())
	if runs["A"] != 1 || runs["B"] != 2 {
		t.Errorf("runs %v", runs)
	}
}

type exitErr int

func (e exitErr) Error() string { return "exit" }
func (e exitErr) ExitCode() int { return int(e) }

func TestRunnerRunsCommandsOnly(t *testing.T) {
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "pat-value-never-on-argv")
	cfg := testConfig(t)
	cfg.commandAuthor = "ana"
	cfg.botArgs = []string{"--knowledge=.okfw"}
	var calls [][]string
	var botEnv []string
	botExit := error(nil)
	fake := func(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
		calls = append(calls, append([]string{name}, args...))
		switch {
		case name == "git" && slices.Contains(args, "clone"):
			_ = os.MkdirAll(filepath.Join(dir, ".git"), 0o700)
		case name == "git" && slices.Contains(args, "rev-parse"):
			return []byte(strings.Repeat("c", 40) + "\n"), nil, nil
		case name == "vizzini-bot":
			botEnv = env
			stderr := []byte("mrman: authorization: {\"decision\":\"deny\",\"input\":{\"event\":\"reply\",\"identity\":\"Vizzini Bot\"}}\n")
			return []byte(`{"answered":["2"],"command_failures":[]}` + "\n"), stderr, botExit
		}
		return nil, nil, nil
	}
	var logs bytes.Buffer
	r := &runner{cfg: cfg, log: slog.New(slog.NewJSONHandler(&logs, nil)), command: fake}
	j := &job{Key: "ado:" + testRepo + "#7", Repo: testRepo, CloneURL: "https://dev.azure.com/org/proj/_git/repo", PRID: 7,
		Target: "main", WebURL: "https://dev.azure.com/org/proj/_git/repo/pullrequest/7", Requester: "ana@example.com"}
	r.run(context.Background(), j)

	var bot []string
	for _, c := range calls {
		for _, a := range c {
			if strings.Contains(a, "pat-value-never-on-argv") {
				t.Errorf("the token is on a command line: %v", c)
			}
		}
		if c[0] == "vizzini-bot" {
			bot = c
		}
	}
	joined := strings.Join(bot, " ")
	for _, want := range []string{"--commands-only", "--base origin/main", "--head " + strings.Repeat("c", 40),
		"--pr ado:dev.azure.com/org/proj/repo#7", "--pr-url https://dev.azure.com/org/proj/_git/repo/pullrequest/7",
		"--grant-policy .vizzini/grant-policy", "--requester ana@example.com", "--max-commands 5", "--command-authors ana", "--knowledge=.okfw"} {
		if !strings.Contains(joined, want) {
			t.Errorf("bot args lack %q: %s", want, joined)
		}
	}
	fetch := strings.Join(calls[1], " ")
	if !strings.Contains(fetch, "+refs/heads/main:refs/remotes/origin/main") || !strings.Contains(fetch, "+refs/pull/7/merge:refs/remotes/pull/7/merge") {
		t.Errorf("fetch: %s", fetch)
	}
	if !slices.ContainsFunc(botEnv, func(e string) bool { return strings.HasPrefix(e, "XDG_DATA_HOME=") }) {
		t.Errorf("no per-job XDG_DATA_HOME")
	}
	if !strings.Contains(logs.String(), `"job done"`) || strings.Contains(logs.String(), "ana@example.com") {
		t.Errorf("logs (requester must be redacted): %s", logs.String())
	}

	// The second job reuses the clone; a denial says what to fix.
	calls, botExit = nil, exitErr(3)
	logs.Reset()
	r.run(context.Background(), j)
	if slices.ContainsFunc(calls, func(c []string) bool { return slices.Contains(c, "clone") }) {
		t.Errorf("cloned again: %v", calls)
	}
	if !strings.Contains(logs.String(), "grant policy denied") || !strings.Contains(logs.String(), "Vizzini Bot") || !strings.Contains(logs.String(), "_allowedIdentities") {
		t.Errorf("denial log: %s", logs.String())
	}
}

func TestParseConfig(t *testing.T) {
	env := map[string]string{"VIZZINI_WEBHOOK_SECRET": testSecret, "AZURE_DEVOPS_EXT_PAT": "p"}
	getenv := func(k string) string { return env[k] }
	ok := []string{"--repo", "https://org@dev.azure.com/org/proj/_git/repo", "--work", t.TempDir()}
	c, err := parseConfig(ok, io.Discard, getenv)
	if err != nil || !c.repos[testRepo] || c.secret != testSecret {
		t.Fatalf("%+v, %v", c, err)
	}
	secretFile := filepath.Join(t.TempDir(), "s")
	_ = os.WriteFile(secretFile, []byte("from-the-file-0123456789\n"), 0o600)
	if c, err := parseConfig(append(ok, "--secret-file", secretFile), io.Discard, getenv); err != nil || c.secret != "from-the-file-0123456789" {
		t.Errorf("secret file: %v", err)
	}
	for name, args := range map[string][]string{
		"no repo":    {"--work", "w"},
		"bad repo":   {"--repo", "github.com/o/r", "--work", "w"},
		"no work":    {"--repo", testRepo},
		"half TLS":   append(slices.Clone(ok), "--tls-cert", "c"),
		"no jobs":    append(slices.Clone(ok), "--max-jobs", "0"),
		"extra args": append(slices.Clone(ok), "x"),
	} {
		if _, err := parseConfig(args, io.Discard, getenv); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	env["VIZZINI_WEBHOOK_SECRET"] = "short"
	if _, err := parseConfig(ok, io.Discard, getenv); err == nil {
		t.Error("short secret accepted")
	}
	env["VIZZINI_WEBHOOK_SECRET"] = testSecret
	delete(env, "AZURE_DEVOPS_EXT_PAT")
	if _, err := parseConfig(ok, io.Discard, getenv); err == nil || !strings.Contains(err.Error(), "AZURE_DEVOPS_EXT_PAT") {
		t.Errorf("no token: %v", err)
	}
	if code := run([]string{"--help"}, io.Discard, getenv); code != 0 {
		t.Errorf("--help: %d", code)
	}
	if code := run([]string{"--work"}, io.Discard, getenv); code != 2 {
		t.Errorf("bad flags: %d", code)
	}
}

func TestRedact(t *testing.T) {
	if redact("ana@example.com") != "ana…" || redact("ab") != "…" {
		t.Error(redact("ana@example.com"), redact("ab"))
	}
}
