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

	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"--mode", "review"}, []string{"## Review", "### [CRITICAL] SQL injection", "`internal/store/users.go:42` · Security"}},
		{[]string{"--mode", "review", "--format", "json"}, []string{`"risk_score": 90`, `"severity": "Critical"`, `"chunks": 1`}},
		{[]string{"--mode", "describe"}, []string{"# Inline the user query", "**Type:** Refactor"}},
		{[]string{"--mode", "describe", "--format", "json"}, []string{`"type": "Refactor"`}},
		{[]string{"--mode", "improve"}, []string{"## Suggested Improvements", "### Bind it"}},
		{[]string{"--mode", "improve", "--format", "json"}, []string{`"suggestions": [`, `"improved_code": "y"`}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			r := runCLI(t, env, patch, tt.args...)
			if r.code != exitOK {
				t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
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
	r := runCLI(t, env, "", "--patch-file", path, "--litellm-url", srv.BaseURL(), "--model", "flag-model", "--format", "json")
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
		{"bad mode", base, []string{"--mode", "explain"}, "--mode must be describe, review or improve"},
		{"bad format", base, []string{"--format", "yaml"}, "--format must be json or markdown"},
		{"stray argument", base, []string{"extra"}, "unexpected arguments"},
		{"unknown flag", base, []string{"--nope"}, "flag provided but not defined"},
		{"missing patch file", base, []string{"--patch-file", "/does/not/exist.patch"}, "no such file"},
		{"describe empty patch", base, []string{"--mode", "describe"}, "no reviewable changes"},
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
