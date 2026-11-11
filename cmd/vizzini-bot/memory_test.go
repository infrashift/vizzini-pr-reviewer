package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

func TestMarkerRoundTrip(t *testing.T) {
	m := marker{FP: "v1:abcd", Run: "r1", Head: "0123456789abcdef0123456789abcdef01234567", Category: "Security", Start: 3, End: 5}
	got, ok := parseMarker("body\n\n" + m.String())
	if !ok || got != m {
		t.Errorf("parsed %+v, want %+v", got, m)
	}
	// Slice 1 markers carry only the fingerprint and run.
	old, ok := parseMarker("x <!-- vizzini:fp=v1:ff run=r0 --> y")
	if !ok || old.FP != "v1:ff" || old.Head != "" || old.Start != 0 {
		t.Errorf("slice-1 marker %+v %v", old, ok)
	}
	if _, ok := parseMarker("no marker <!-- other -->"); ok {
		t.Error("parsed a foreign comment")
	}
}

func TestMapRange(t *testing.T) {
	for _, tc := range []struct {
		name       string
		diff       string
		state      anchorState
		start, end int
	}{
		{"no change", "", anchorSame, 10, 12},
		{"lines added above", "@@ -2,0 +3,4 @@\n+a\n+b\n+c\n+d\n", anchorSame, 14, 16},
		{"lines removed above", "@@ -2,3 +1,0 @@\n-a\n-b\n-c\n", anchorSame, 7, 9},
		{"replaced above", "@@ -1,2 +1,5 @@\n", anchorSame, 13, 15},
		{"changed below", "@@ -20,2 +20,9 @@\n", anchorSame, 10, 12},
		{"inserted right after the range", "@@ -12,0 +13,2 @@\n", anchorSame, 10, 12},
		{"the anchor line changed", "@@ -11 +11 @@\n-x\n+y\n", anchorChanged, 0, 0},
		{"a change overlapping the start", "@@ -8,3 +8,1 @@\n", anchorChanged, 0, 0},
		{"inserted inside the range", "@@ -10,0 +11,1 @@\n", anchorChanged, 0, 0},
		{"file deleted", "diff --git a/x b/x\ndeleted file mode 100644\n--- a/x\n+++ /dev/null\n@@ -1,20 +0,0 @@\n", anchorChanged, 0, 0},
	} {
		state, start, end := mapRange([]byte(tc.diff), 10, 12)
		if state != tc.state || start != tc.start || end != tc.end {
			t.Errorf("%s: %v %d-%d, want %v %d-%d", tc.name, state, start, end, tc.state, tc.start, tc.end)
		}
	}
}

func TestReconcile(t *testing.T) {
	c := func(file, cat, fp string, s, e int) comment {
		return comment{File: file, Category: cat, Fingerprint: fp, StartLine: s, EndLine: e}
	}
	th := func(id, path, cat, fp string, resolved bool, anchor anchorState, s, e int) botThread {
		return botThread{ID: id, Path: path, Resolved: resolved, Anchor: anchor, Start: s, End: e,
			Marker: marker{FP: fp, Category: cat}}
	}
	comments := []comment{
		c("a.cs", "Security", "fp-same", 10, 10),         // same fingerprint as thread 1
		c("a.cs", "Logic", "fp-reworded", 30, 31),        // reworded title, same place as thread 2
		c("b.cs", "Security", "fp-new", 5, 5),            // new
		c("c.cs", "Security", "fp-regressed", 7, 7),      // thread 6 was fixed: a recurrence is new
		c("a.cs", "Performance", "fp-other-cat", 30, 30), // overlaps thread 2, other category: new
	}
	threads := []botThread{
		th("1", "a.cs", "Security", "fp-same", false, anchorChanged, 0, 0),
		th("2", "a.cs", "Logic", "fp-old-title", false, anchorSame, 29, 31),
		th("3", "d.cs", "Logic", "fp-gone-fixed", false, anchorChanged, 0, 0),   // gone, code changed: resolve
		th("4", "d.cs", "Logic", "fp-gone-same", false, anchorSame, 3, 3),       // gone, code same: keep
		th("5", "d.cs", "Logic", "fp-gone-unknown", false, anchorUnknown, 0, 0), // gone, unknown: keep
		th("6", "c.cs", "Security", "fp-regressed", true, anchorChanged, 0, 0),  // fixed earlier
		th("7", "e.cs", "Logic", "fp-dismissed", true, anchorSame, 1, 1),        // resolved by a person: leave
	}
	r := reconcile(comments, threads, nil)
	posted := make([]string, 0, len(r.Post))
	for _, p := range r.Post {
		posted = append(posted, p.Fingerprint)
	}
	if !slices.Equal(posted, []string{"fp-new", "fp-regressed", "fp-other-cat"}) {
		t.Errorf("post %v", posted)
	}
	if !slices.Equal(r.AlreadyPosted, []string{"fp-same", "fp-reworded"}) {
		t.Errorf("already posted %v", r.AlreadyPosted)
	}
	if !slices.Equal(r.Resolve, []string{"3"}) || !slices.Equal(r.KeptOpen, []string{"4", "5"}) {
		t.Errorf("resolve %v kept %v", r.Resolve, r.KeptOpen)
	}
}

// A finding a person resolved (won't fix) is not posted again.
func TestReconcile_DismissedFindingStaysDismissed(t *testing.T) {
	r := reconcile([]comment{{File: "a.cs", Category: "Logic", Fingerprint: "fp", StartLine: 4, EndLine: 4}},
		[]botThread{{ID: "9", Path: "a.cs", Resolved: true, Anchor: anchorSame, Start: 4, End: 4, Marker: marker{FP: "fp", Category: "Logic"}}}, nil)
	if len(r.Post) != 0 || len(r.Resolve) != 0 {
		t.Errorf("%+v", r)
	}
}

// End to end over a real repository: post, re-run without changes (nothing
// new), fix the code (the thread resolves).
func TestBot_ThreadMemoryAcrossPushes(t *testing.T) {
	repo := gitRepo(t)
	reply := `{"summary":"Injection.","risk_score":90,"findings":[` +
		`{"file":"store.go","start_line":3,"end_line":3,"severity":"Critical","category":"Security","title":"%s","body":"b"}]}`
	title := "SQL injection"
	srv := llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		if title == "" {
			return llmtest.Reply{Content: `{"summary":"Clean.","risk_score":5,"findings":[]}`}
		}
		return llmtest.Reply{Content: strings.Replace(reply, "%s", title, 1)}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	args := []string{"--repo", repo, "--base", "main", "--pr-url", "u", "--grant-policy", "p"}

	// Run 1: nothing posted yet.
	f := &fakeMrman{submitOut: `{"submitted":true}`}
	f.install(t)
	code, stdout, stderr := runBot(t, env, args...)
	if code != exitOK {
		t.Fatalf("run 1: exit %d: %s", code, stderr)
	}
	body := f.calls[2][len(f.calls[2])-1]
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Posted != 1 {
		t.Fatalf("run 1 result %s (%v)", stdout, err)
	}

	// Run 2 again with --full, the model rewords the title: nothing new, nothing resolved.
	thread := func(resolved bool) string {
		b, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
			"id": "77", "path": "/store.go", "is_resolved": resolved, "comments": []any{map[string]any{"body": body}},
		}}})
		return string(b)
	}
	// Run 2, same head: by default nothing changed, so no review at all.
	before := len(srv.Requests())
	f = &fakeMrman{threadsOut: thread(false)}
	f.install(t)
	code, stdout, _ = runBot(t, env, args...)
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil || res.Mode != "unchanged" ||
		len(srv.Requests()) != before || len(f.calls) != 2 {
		t.Fatalf("run 2 (incremental): exit %d, result %s, %d requests, calls %v", code, stdout, len(srv.Requests())-before, f.calls)
	}

	title = "Query built by string concatenation"
	f = &fakeMrman{threadsOut: thread(false)}
	f.install(t)
	code, stdout, stderr = runBot(t, env, append(args, "--full")...)
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil || res.Posted != 0 ||
		len(res.AlreadyPosted) != 1 || len(f.calls) != 2 {
		t.Fatalf("run 2: exit %d, result %s, calls %v: %s", code, stdout, f.calls, stderr)
	}

	// Run 2b, --full at the same head, the model misses it: the thread stays open.
	title = ""
	f = &fakeMrman{threadsOut: thread(false)}
	f.install(t)
	_, stdout, _ = runBot(t, env, append(args, "--full")...)
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || !slices.Equal(res.KeptOpen, []string{"77"}) || len(f.calls) != 2 {
		t.Fatalf("run 2b: result %s, calls %v", stdout, f.calls)
	}

	// Run 3: a push fixes line 3 and the finding is gone: an incremental
	// review of store.go resolves thread 77.
	if err := os.WriteFile(filepath.Join(repo, "store.go"),
		[]byte("package store\n\n// Bound parameter.\nconst q = \"SELECT * FROM users WHERE name = $1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git := exec.CommandContext(t.Context(), "git", "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qam", "fix")
	git.Dir = repo
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	f = &fakeMrman{threadsOut: thread(false), resolveOut: `{"resolved":["77"],"already_resolved":[],"failed":[]}`}
	f.install(t)
	code, stdout, stderr = runBot(t, env, args...)
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil || !slices.Equal(res.Resolved, []string{"77"}) ||
		res.Mode != "incremental" || !slices.Equal(res.ReviewedFiles, []string{"store.go"}) || len(res.Since) != 40 {
		t.Fatalf("run 3: exit %d, result %s: %s", code, stdout, stderr)
	}
	resolve := strings.Join(f.calls[len(f.calls)-1], " ")
	for _, want := range []string{"review resolve", "--thread 77", "--grant-policy", "--run-id", "--agent vizzini-bot", "no longer reports this", "<!-- vizzini:reviewed head="} {
		if !strings.Contains(resolve, want) {
			t.Errorf("resolve call %q lacks %q", resolve, want)
		}
	}

	// A policy that denies resolve: exit 3.
	f = &fakeMrman{threadsOut: thread(false), resolveOut: `{"error":"policy_denied"}`, resolveErr: errExit1}
	f.install(t)
	if code, _, _ := runBot(t, env, args...); code != exitDenied {
		t.Errorf("denied resolve: exit %d", code)
	}
}

var errExit1 = errors.New("exit status 1")
