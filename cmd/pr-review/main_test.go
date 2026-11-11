package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

func TestMain(m *testing.M) { llmtest.RunOffline(m) }

const patch = `diff --git a/internal/store/users.go b/internal/store/users.go
--- a/internal/store/users.go
+++ b/internal/store/users.go
@@ -41,3 +41,3 @@ func FindUser() {
 	if name == "" {
-	row := db.QueryRow("SELECT * FROM users WHERE name = $1", name)
+	row := db.QueryRow("SELECT * FROM users WHERE name = '" + name + "'")
 	var u User
`

func mockProxy(t *testing.T) *llmtest.Server {
	t.Helper()
	return llmtest.NewServer(t, func(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
		sys := req.Messages[0].Content
		switch {
		case strings.Contains(sys, "writing the description"):
			return llmtest.Reply{Content: `{"title":"Inline the user query","type":"Refactor","summary":"s","changes":[{"file":"internal/store/users.go","description":"d"}]}`}
		case strings.Contains(sys, "suggesting concrete improvements"):
			return llmtest.Reply{Content: `{"suggestions":[{"file":"internal/store/users.go","start_line":42,"end_line":42,"category":"Security","summary":"Bind it","existing_code":"x","improved_code":"y"}]}`}
		default:
			return llmtest.Reply{Content: `{"summary":"Injection.","risk_score":90,"findings":[{"file":"internal/store/users.go","start_line":42,"end_line":42,"severity":"Critical","category":"Security","title":"SQL injection","body":"b"}]}`}
		}
	})
}

type result struct {
	code           int
	stdout, stderr string
}

func runCLI(t *testing.T, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errb, func(k string) string { return env[k] })
	return result{code, out.String(), errb.String()}
}

func TestRun_ModesAndFormats(t *testing.T) {
	srv := mockProxy(t)
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m", "VIZZINI_API_KEY": "k"}

	// The mock review has a Critical finding, so review mode trips the default
	// policy gate (exit 2) while still printing its full output.
	tests := []struct {
		args     []string
		wantCode int
		want     []string
	}{
		{[]string{"--mode", "review"}, exitPolicy, []string{"## Review", "### [CRITICAL] SQL injection", "`internal/store/users.go:42` · Security"}},
		{[]string{"--mode", "review", "--format", "json"}, exitPolicy, []string{`"apiVersion": "vizzini.infrashift.io/v1"`, `"kind": "Review"`, `"risk_score": 90`, `"severity": "Critical"`, `"chunks": 1`}},
		{[]string{"--mode", "describe"}, exitOK, []string{"# Inline the user query", "**Type:** Refactor"}},
		{[]string{"--mode", "describe", "--format", "json"}, exitOK, []string{`"type": "Refactor"`}},
		{[]string{"--mode", "improve"}, exitOK, []string{"## Suggested Improvements", "### Bind it"}},
		{[]string{"--mode", "improve", "--format", "json"}, exitOK, []string{`"kind": "Suggestions"`, `"suggestions": [`, `"improved_code": "y"`}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			r := runCLI(t, env, patch, tt.args...)
			if r.code != tt.wantCode {
				t.Fatalf("exit %d, want %d; stderr: %s", r.code, tt.wantCode, r.stderr)
			}
			for _, w := range tt.want {
				if !strings.Contains(r.stdout, w) {
					t.Errorf("stdout lacks %q:\n%s", w, r.stdout)
				}
			}
			if strings.Contains(tt.args[len(tt.args)-1], "json") && !json.Valid([]byte(r.stdout)) {
				t.Errorf("stdout is not valid JSON:\n%s", r.stdout)
			}
		})
	}
}

func TestRun_PatchFileAndFlagsOverrideEnv(t *testing.T) {
	srv := mockProxy(t)
	path := filepath.Join(t.TempDir(), "change.patch")
	if err := os.WriteFile(path, []byte(patch), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"VIZZINI_LITELLM_URL": "http://127.0.0.1:1/v1", "VIZZINI_MODEL": "env-model", "LITELLM_API_KEY": "fallback-key"}
	r := runCLI(t, env, "", "--patch-file", path, "--litellm-url", srv.BaseURL(), "--model", "flag-model", "--format", "json", "--fail-on", "none")
	if r.code != exitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if got := srv.Requests()[0].Model; got != "flag-model" {
		t.Errorf("model %q, want the flag value", got)
	}
}

func TestRun_Errors(t *testing.T) {
	srv := mockProxy(t)
	base := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	tests := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"missing url", map[string]string{"VIZZINI_MODEL": "m"}, nil, "--litellm-url (or VIZZINI_LITELLM_URL) is required"},
		{"missing model", map[string]string{"VIZZINI_LITELLM_URL": "http://x/v1"}, nil, "--model (or VIZZINI_MODEL) is required"},
		{"bad mode", base, []string{"--mode", "explain"}, "--mode must be describe, review, improve, command or plan"},
		{"bad format", base, []string{"--format", "yaml"}, "--format must be json or markdown"},
		{"stray argument", base, []string{"extra"}, "unexpected arguments"},
		{"unknown flag", base, []string{"--nope"}, "flag provided but not defined"},
		{"missing patch file", base, []string{"--patch-file", "/does/not/exist.patch"}, "no such file"},
		{"describe empty patch", base, []string{"--mode", "describe"}, "no reviewable changes"},
		{"unknown fail-on severity", base, []string{"--fail-on", "High"}, `--fail-on: schema validation failed: #PolicyGate.fail_on.0: conflicting values "Critical" and "High"`},
		{"fail-on outside review", base, []string{"--mode", "improve", "--fail-on", "Minor"}, "--fail-on applies only to --mode review"},
		{"lint-json outside review", base, []string{"--mode", "describe", "--lint-json", "x.json"}, "--lint-json applies only to --mode review"},
		{"lint-json from stdin", base, []string{"--lint-json", "-"}, "--lint-json must be a file"},
		{"lint-json missing", base, []string{"--lint-json", "/does/not/exist.json"}, "--lint-json: open /does/not/exist.json"},
		{"command without --command", base, []string{"--mode", "command"}, "--command and --mode command go together"},
		{"--command without mode", base, []string{"--command", "/explain a.go:1"}, "--command and --mode command go together"},
		{"thread outside command", base, []string{"--thread", "t.json"}, "--thread applies only to --mode command"},
		{"bad command", base, []string{"--mode", "command", "--command", "/summarize x.go:1"}, "unknown command /summarize"},
		{"command out of scope", base, []string{"--mode", "command", "--command", "/explain nope.go:1"}, "nope.go is not part of this diff"},
		{"plan without issue", base, []string{"--mode", "plan"}, "--issue and --mode plan go together"},
		{"tree outside plan", base, []string{"--tree", "t.txt"}, "--tree and --okf-index apply only to --mode plan"},
		{"issue missing", base, []string{"--mode", "plan", "--issue", "/does/not/exist.json"}, "--issue: open /does/not/exist.json"},
		{"knowledge outside review", base, []string{"--mode", "improve", "--knowledge", "."}, "--knowledge applies only to --mode review"},
		{"knowledge missing", base, []string{"--knowledge", "/does/not/exist"}, "--knowledge: stat /does/not/exist"},
		{"knowledge not a dir", base, []string{"--knowledge", "main.go"}, "--knowledge: main.go is not a directory"},
		{"risk-policy missing", base, []string{"--risk-policy", "/does/not/exist.json"}, "--risk-policy: open /does/not/exist.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := patch
			if tt.name == "describe empty patch" {
				stdin = ""
			}
			r := runCLI(t, tt.env, stdin, tt.args...)
			if r.code != exitError {
				t.Errorf("exit %d, want %d (exit 2 is reserved for the policy gate)", r.code, exitError)
			}
			if !strings.Contains(r.stderr, tt.want) {
				t.Errorf("stderr %q lacks %q", r.stderr, tt.want)
			}
		})
	}
}

func TestRun_Help(t *testing.T) {
	r := runCLI(t, nil, "", "-h")
	if r.code != exitOK || !strings.Contains(r.stderr, "Usage: pr-review") || !strings.Contains(r.stderr, "VIZZINI_API_KEY") {
		t.Errorf("exit %d, stderr:\n%s", r.code, r.stderr)
	}
}

// reviewWith answers every review with one finding of the given severity, or
// none when sev is empty.
func reviewWith(t *testing.T, sev string) *llmtest.Server {
	t.Helper()
	findings := "[]"
	if sev != "" {
		findings = `[{"file":"internal/store/users.go","start_line":42,"end_line":42,"severity":"` + sev + `","category":"Logic","title":"t","body":"b"}]`
	}
	return llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":10,"findings":` + findings + `}`}
	})
}

func TestRun_PolicyGateExitCodes(t *testing.T) {
	tests := []struct {
		finding string
		failOn  []string // nil: default gate
		want    int
	}{
		{"Critical", nil, exitPolicy},
		{"Major", nil, exitPolicy},
		{"Minor", nil, exitOK},
		{"Trivial", nil, exitOK},
		{"", nil, exitOK},
		{"Minor", []string{"--fail-on", "Minor"}, exitPolicy},
		{"Critical", []string{"--fail-on", "Minor"}, exitOK},
		{"Trivial", []string{"--fail-on", "Critical,Major,Minor,Trivial"}, exitPolicy},
		{"Major", []string{"--fail-on", " Critical , Major "}, exitPolicy},
		{"Critical", []string{"--fail-on", "none"}, exitOK},
		{"Critical", []string{"--fail-on", ""}, exitOK},
	}
	for _, tt := range tests {
		name := tt.finding + " " + strings.Join(tt.failOn, "=")
		t.Run(name, func(t *testing.T) {
			srv := reviewWith(t, tt.finding)
			env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
			r := runCLI(t, env, patch, tt.failOn...)
			if r.code != tt.want {
				t.Fatalf("exit %d, want %d; stderr: %s", r.code, tt.want, r.stderr)
			}
			if !strings.Contains(r.stdout, "## Review") {
				t.Error("review output missing; the gate must not suppress it")
			}
			gateMsg := strings.Contains(r.stderr, "policy gate failed: 1 finding(s) at")
			if gateMsg != (tt.want == exitPolicy) {
				t.Errorf("stderr %q: gate message present = %v, want %v", r.stderr, gateMsg, tt.want == exitPolicy)
			}
		})
	}
}

// An invalid gate is rejected before any tokens are spent.
func TestRun_InvalidGateMakesNoRequests(t *testing.T) {
	srv := reviewWith(t, "Critical")
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	if r := runCLI(t, env, patch, "--fail-on", "Blocker"); r.code != exitError {
		t.Fatalf("exit %d, want %d", r.code, exitError)
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d LLM requests made despite an invalid --fail-on", n)
	}
}

func TestRun_LintJSON(t *testing.T) {
	srv := mockProxy(t)
	path := filepath.Join(t.TempDir(), "lint.json")
	report := `{"Issues":[{"FromLinter":"gosec","Text":"G202: SQL string concatenation","Pos":{"Filename":"internal/store/users.go","Line":42,"Column":9}}]}`
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	r := runCLI(t, env, patch, "--lint-json", path, "--format", "json", "--fail-on", "none")
	if r.code != exitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, `"diagnostics": 1`) {
		t.Errorf("JSON output lacks the diagnostics count:\n%s", r.stdout)
	}
	if p := llmtest.UserPrompt(&srv.Requests()[0]); !strings.Contains(p, "internal/store/users.go:42:9 gosec: G202") {
		t.Errorf("prompt lacks the diagnostic:\n%s", p)
	}
}

func TestRun_RiskPolicyAndAutoApprove(t *testing.T) {
	srv := reviewWith(t, "")
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}

	r := runCLI(t, env, patch, "--format", "json")
	if r.code != exitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{`"auto_approve": true`, `"base_risk_score": `, `"model_risk_score": 10`, `"risk_files": [`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("JSON lacks %s:\n%s", want, r.stdout)
		}
	}

	dir := t.TempDir()
	strict := filepath.Join(dir, "strict.json")
	if err := os.WriteFile(strict, []byte(`{"auto_approve_threshold": 0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r = runCLI(t, env, patch, "--format", "json", "--risk-policy", strict)
	if r.code != exitOK || !strings.Contains(r.stdout, `"auto_approve": false`) {
		t.Errorf("strict policy: exit %d, stdout:\n%s", r.code, r.stdout)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"scale": 0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r = runCLI(t, env, patch, "--risk-policy", bad)
	if r.code != exitError || !strings.Contains(r.stderr, "risk policy: schema validation failed") {
		t.Errorf("invalid policy: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestRun_Knowledge(t *testing.T) {
	srv := reviewWith(t, "")
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	r := runCLI(t, env, patch, "--knowledge", "../../internal/okf/testdata/okf", "--format", "json")
	if r.code != exitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, `"knowledge_docs": [`) || !strings.Contains(r.stdout, `"adr-0001-store"`) {
		t.Errorf("knowledge_docs missing:\n%s", r.stdout)
	}
	if p := llmtest.UserPrompt(&srv.Requests()[0]); !strings.Contains(p, "<knowledge>") {
		t.Error("prompt lacks the knowledge block")
	}
}

func TestRun_CommandMode(t *testing.T) {
	srv := llmtest.NewServer(t, func(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"answer":"It builds SQL from input."}`}
	})
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	thread := filepath.Join(t.TempDir(), "thread.json")
	if err := os.WriteFile(thread, []byte(`[{"author":"alice","body":"Safe?"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runCLI(t, env, patch, "--mode", "command", "--command", "/explain internal/store/users.go:42 safe?", "--thread", thread)
	if r.code != exitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if r.stdout != "**/explain** `internal/store/users.go:42`: safe?\n\nIt builds SQL from input.\n" {
		t.Errorf("stdout %q", r.stdout)
	}
	if !strings.Contains(llmtest.UserPrompt(&srv.Requests()[0]), "alice: Safe?") {
		t.Error("thread not sent")
	}
	r = runCLI(t, env, patch, "--mode", "command", "--command", "/explain internal/store/users.go:42", "--format", "json")
	if r.code != exitOK || !strings.Contains(r.stdout, `"answer": "It builds SQL from input."`) {
		t.Errorf("json: exit %d stdout %s", r.code, r.stdout)
	}
}

func TestRun_PlanMode(t *testing.T) {
	plan := `{"summary":"s","affected_files":[{"path":"a.go","change":"modify","reason":"r"}],"phases":[{"name":"p","steps":[{"title":"t","description":"d","files":["a.go"]}]}],"verification":["make test"]}`
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply { return llmtest.Reply{Content: plan} })
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	dir := t.TempDir()
	issue, tree := filepath.Join(dir, "issue.json"), filepath.Join(dir, "tree.txt")
	if err := os.WriteFile(issue, []byte(`{"title":"Fix a","body":"b","labels":["bug"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tree, []byte("a.go\n\nb.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// stdin is not read in plan mode: an unreadable patch source must not matter.
	r := runCLI(t, env, "not a patch", "--mode", "plan", "--issue", issue, "--tree", tree)
	if r.code != exitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "# Plan: Fix a\n") || !strings.Contains(r.stdout, "- [ ] **t**: d (`a.go`)") {
		t.Errorf("stdout:\n%s", r.stdout)
	}
	if p := llmtest.UserPrompt(&srv.Requests()[0]); !strings.Contains(p, "<tree>\na.go\nb.go\n</tree>") {
		t.Errorf("tree not sent:\n%s", p)
	}
}
