package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

// suppressRepo: main carries a rule for store.go; the feature branch (the
// PR) adds its own rule for store.go and changes line 3.
func suppressRepo(t *testing.T, baseRules string) string {
	t.Helper()
	dir, git, write := scopeRepo(t) // main: a.go, b.go; feature changes both
	git("checkout", "-q", "main")
	if baseRules != "" {
		if err := os.MkdirAll(filepath.Join(dir, ".vizzini"), 0o750); err != nil {
			t.Fatal(err)
		}
		write(".vizzini/suppressions.yaml", baseRules)
		git("add", ".vizzini")
		git("commit", "-q", "-m", "rules")
	}
	git("checkout", "-q", "feature")
	git("merge", "-q", "--no-edit", "main")
	if err := os.MkdirAll(filepath.Join(dir, ".vizzini"), 0o750); err != nil {
		t.Fatal(err)
	}
	// The PR tries to silence its own b.go finding: it must not work.
	write(".vizzini/suppressions.yaml", "suppressions:\n  - {id: SELF-SERVE, reason: mine, paths: [b.go]}\n")
	git("add", ".vizzini")
	git("commit", "-q", "-m", "silence my own finding")
	return dir
}

func TestLoadSuppressions_FromTheBaseOnly(t *testing.T) {
	dir := suppressRepo(t, "suppressions:\n  - {id: A-OK, reason: accepted, paths: [a.go]}\n")
	opts := &options{repo: dir, base: "main", suppressions: ".vizzini/suppressions.yaml"}
	rules, err := loadSuppressions(t.Context(), opts, &strings.Builder{})
	if err != nil || rules.Len() != 1 {
		t.Fatalf("rules %d, %v", rules.Len(), err)
	}
	if rules.Match(&reviewer.Finding{File: "b.go"}, nowUTC()) != nil {
		t.Error("the PR's own rule must not apply")
	}

	// No rules file on the base: no rules. An invalid one fails the run.
	dir = suppressRepo(t, "")
	if rules, err := loadSuppressions(t.Context(), &options{repo: dir, base: "main", suppressions: ".vizzini/suppressions.yaml"},
		&strings.Builder{}); err != nil || rules != nil {
		t.Errorf("missing file: %v %v", rules, err)
	}
	dir = suppressRepo(t, "suppressions:\n  - {id: X, reason: r, categories: [Logic]}\n")
	if _, err := loadSuppressions(t.Context(), &options{repo: dir, base: "main", suppressions: ".vizzini/suppressions.yaml"},
		&strings.Builder{}); err == nil || !strings.Contains(err.Error(), "main:.vizzini/suppressions.yaml") {
		t.Errorf("invalid rules must fail the run, got %v", err)
	}
	if rules, err := loadSuppressions(t.Context(), &options{repo: dir, base: "main"}, &strings.Builder{}); err != nil || rules != nil {
		t.Errorf("--suppressions \"\" disables: %v %v", rules, err)
	}
}

// End to end: a suppressed finding is not posted, and the open thread that
// raised it (whose code changed) is left alone instead of resolved.
func TestBot_SuppressedFindingIsNotPostedAndItsThreadIsLeftAlone(t *testing.T) {
	dir := suppressRepo(t, "suppressions:\n  - {id: A-OK, reason: accepted risk, paths: [a.go], categories: [Security]}\n")
	srv := mockFindings(t, `{"file":"a.go","start_line":3,"end_line":3,"severity":"Critical","category":"Security","title":"Bad A","body":"b"},`+
		`{"file":"b.go","start_line":3,"end_line":3,"severity":"Minor","category":"Logic","title":"Bad B","body":"b"}`)
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	// An open thread raising the same finding (same fingerprint) from an
	// earlier head this checkout does not have.
	fp := reviewer.Fingerprint(&reviewer.Finding{File: "a.go", Category: "Security", Title: "Bad A"})
	m := marker{FP: fp, Run: "r0", Head: strings.Repeat("ab", 20), Category: "Security", Start: 3, End: 3}
	threads, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
		"id": "31", "path": "a.go", "comments": []any{map[string]any{"id": "1", "body": "x\n\n" + m.String()}}}}})
	f := &fakeMrman{submitOut: `{}`, threadsOut: string(threads)}
	f.install(t)
	code, stdout, stderr := runBot(t, env, "--repo", dir, "--base", "main", "--pr-url", "u", "--grant-policy", "p", "--full")
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil {
		t.Fatalf("exit %d, %s: %s", code, stdout, stderr)
	}
	// Thread 31 is matched by the suppressed finding, so it is neither
	// kept open (as an unreported finding) nor resolved.
	if !slices.Equal(res.Suppressed, []string{"A-OK a.go:3"}) || res.Posted != 1 || len(res.Resolved) != 0 ||
		len(res.KeptOpen) != 0 || len(res.AlreadyPosted) != 0 {
		t.Errorf("result %s", stdout)
	}
	for _, c := range f.calls {
		if c[1] == "add" && slices.Contains(c, "a.go") {
			t.Errorf("a suppressed finding was drafted: %v", c)
		}
	}
}

func TestDismissalSuggestion(t *testing.T) {
	m := marker{FP: "v1:0123456789abcdef", Run: "r0", Head: strings.Repeat("ab", 20), Category: "Maintainability", Start: 7, End: 7}
	thread := func(id, disposition string, resolved bool, extra ...remoteComment) remoteThread {
		return remoteThread{ID: id, Path: "src/Legacy/ReportFormatter.cs", IsResolved: resolved, Disposition: disposition,
			Comments: append([]remoteComment{{ID: "1", Body: "**Minor (Maintainability): Unused variable \"width\"**\n\nIt is never read.\n\n" + m.String()}}, extra...)}
	}
	threads := []remoteThread{
		thread("1", "won't fix", true),
		thread("2", "by design", true),
		thread("3", "resolved", true), // fixed, not dismissed
		thread("4", "", false),        // open
		thread("5", "won't fix", true, remoteComment{ID: "9", Body: "<!-- vizzini:suggested-suppression thread=5 run=r head=abc -->"}),
		{ID: "6", Path: "x.cs", IsResolved: true, Disposition: "won't fix", Comments: []remoteComment{{ID: "1", Body: "a person's thread"}}},
	}
	found := dismissals(threads)
	ids := make([]string, 0, len(found))
	for _, t := range found {
		ids = append(ids, t.ID)
	}
	if !slices.Equal(ids, []string{"1", "2"}) {
		t.Fatalf("dismissals %v, want 1 and 2", ids)
	}

	opts := &options{suppressions: ".vizzini/suppressions.yaml", attr: reviewer.Attribution{RunID: "r1", Iteration: strings.Repeat("cd", 20), PRKey: "ado:o/p/r#7"}}
	body := suggestion(&threads[0], opts)
	for _, want := range []string{"dismissed (won't fix)", "`.vizzini/suppressions.yaml` on the base branch", `paths: ["src/Legacy/ReportFormatter.cs"]`,
		"categories: [Maintainability]", `# title: "Unused variable \"width\""`, "ado:o/p/r#7",
		"<!-- vizzini:suggested-suppression thread=1 run=r1 head="} {
		if !strings.Contains(body, want) {
			t.Errorf("suggestion lacks %q:\n%s", want, body)
		}
	}
	// The suggested rule is a valid rule file as it stands, and it matches
	// the dismissed finding.
	yaml := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindStringSubmatch(body)
	if yaml == nil {
		t.Fatalf("no yaml block:\n%s", body)
	}
	rules, err := reviewer.ParseSuppressions([]byte(yaml[1]))
	if err != nil {
		t.Fatalf("the suggested rule does not parse: %v\n%s", err, yaml[1])
	}
	if rules.Match(&reviewer.Finding{File: "src/Legacy/ReportFormatter.cs", Category: "Maintainability", Title: "width unused"}, nowUTC()) == nil {
		t.Error("the suggested rule does not match the dismissed finding")
	}
}

func nowUTC() time.Time { return time.Now().UTC() }

// mockFindings answers every review with these findings (JSON objects,
// comma-separated) and every command with a short explanation.
func mockFindings(t *testing.T, findings string) *llmtest.Server {
	t.Helper()
	return llmtest.NewServer(t, func(_ int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":50,"findings":[` + findings + `]}`}
	})
}
