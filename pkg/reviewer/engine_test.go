package reviewer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

func TestMain(m *testing.M) { llmtest.RunOffline(m) }

func fixture(t *testing.T) *bytes.Reader {
	t.Helper()
	b, err := os.ReadFile("testdata/change.patch")
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func newEngine(t *testing.T, srv *llmtest.Server, mutate ...func(*Config)) *Engine {
	t.Helper()
	cfg := Config{BaseURL: srv.BaseURL(), APIKey: "sk-test", Model: "test-model", Backoff: time.Millisecond}
	for _, f := range mutate {
		f(&cfg)
	}
	e, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v (run `make tiktoken-cache` if the tokenizer failed to load)", err)
	}
	return e
}

const (
	sqliReview = `{"summary":"Query is now built by concatenation.","risk_score":92,"findings":[
		{"file":"internal/store/users.go","start_line":44,"end_line":45,"severity":"Critical","category":"Security",
		 "title":"SQL injection via string-built query","body":"name is concatenated into SQL.","suggestion":"use $1"}]}`
	handlerReview = `{"summary":"New handler ignores the form error.","risk_score":30,"findings":[
		{"file":"internal/api/handler.go","start_line":4,"end_line":4,"severity":"Minor","category":"Logic",
		 "title":"Form parse error ignored","body":"ParseForm can fail."}]}`
)

// answerByFile replies with the review for whichever fixture files appear in
// the prompt, merged when a chunk holds both.
func answerByFile(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
	p := llmtest.UserPrompt(req)
	users := strings.Contains(p, "### internal/store/users.go")
	handler := strings.Contains(p, "### internal/api/handler.go")
	switch {
	case users && handler:
		return llmtest.Reply{Content: `{"summary":"Two changes.","risk_score":92,"findings":[
			{"file":"internal/api/handler.go","start_line":4,"end_line":4,"severity":"Minor","category":"Logic","title":"Form parse error ignored","body":"ParseForm can fail."},
			{"file":"internal/store/users.go","start_line":44,"end_line":45,"severity":"Critical","category":"Security","title":"SQL injection via string-built query","body":"name is concatenated into SQL.","suggestion":"use $1"}]}`}
	case users:
		return llmtest.Reply{Content: sqliReview}
	case handler:
		return llmtest.Reply{Content: handlerReview}
	}
	return llmtest.Reply{Content: `{"summary":"nothing","risk_score":0,"findings":[]}`}
}

func TestReview_EndToEnd(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	out, err := newEngine(t, srv).Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if out.Chunks != 1 || len(srv.Requests()) != 1 {
		t.Errorf("chunks %d, requests %d; want 1 and 1", out.Chunks, len(srv.Requests()))
	}
	if out.RiskScore != 92 || out.Summary != "Two changes." {
		t.Errorf("risk %d summary %q", out.RiskScore, out.Summary)
	}
	if !reflect.DeepEqual(out.Excluded, []string{"go.sum"}) {
		t.Errorf("excluded %v, want [go.sum]", out.Excluded)
	}
	if len(out.Findings) != 2 || out.Findings[0].Severity != SeverityCritical || out.Findings[1].Severity != SeverityMinor {
		t.Fatalf("findings not sorted most severe first: %+v", out.Findings)
	}

	req := srv.Requests()[0]
	if req.Model != "test-model" || req.ResponseFormat == nil || req.ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
		t.Errorf("request model %q format %+v", req.Model, req.ResponseFormat)
	}
	prompt := llmtest.UserPrompt(&req)
	if strings.Contains(prompt, "go.sum") {
		t.Error("excluded go.sum reached the prompt")
	}
	if !strings.Contains(prompt, "   44 + \tq := \"SELECT") {
		t.Errorf("prompt lacks annotated line numbers:\n%s", prompt)
	}
}

func TestReview_MultiChunkMerge(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	// A budget that fits one file but not both forces two chunks.
	out, err := newEngine(t, srv, func(c *Config) { c.MaxChunkTokens = 150 }).Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Chunks != 2 || len(srv.Requests()) != 2 {
		t.Fatalf("chunks %d, requests %d; want 2 and 2", out.Chunks, len(srv.Requests()))
	}
	if out.RiskScore != 92 {
		t.Errorf("risk %d, want the max (92)", out.RiskScore)
	}
	if !strings.HasPrefix(out.Summary, "Part 1 of 2: ") || !strings.Contains(out.Summary, "\n\nPart 2 of 2: ") {
		t.Errorf("summary %q", out.Summary)
	}
	if len(out.Findings) != 2 || out.Findings[0].File != "internal/store/users.go" {
		t.Errorf("findings %+v", out.Findings)
	}
	for _, req := range srv.Requests() {
		if !strings.Contains(llmtest.UserPrompt(&req), "of 2; other parts are reviewed separately") {
			t.Error("chunk prompt does not say it is one of two parts")
		}
	}
}

func TestReview_AnchorsFindingsToTheDiff(t *testing.T) {
	tests := []struct {
		name      string
		first     string
		wantInMsg string
	}{
		{
			"file not in diff",
			strings.Replace(sqliReview, "internal/store/users.go", "internal/store/orders.go", 1),
			`file "internal/store/orders.go" is not part of this diff`,
		},
		{
			"lines outside hunks",
			strings.Replace(strings.Replace(sqliReview, `"start_line":44`, `"start_line":120`, 1), `"end_line":45`, `"end_line":121`, 1),
			"lines 120-121 of internal/store/users.go are outside the changed hunks (new lines 41-47)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := llmtest.NewServer(t, func(n int, req *openai.ChatCompletionRequest) llmtest.Reply {
				if n == 1 {
					return llmtest.Reply{Content: tt.first}
				}
				return llmtest.Reply{Content: sqliReview}
			})
			// Only users.go: its hunk covers new lines 41-47.
			patch, _, _ := strings.Cut(mustRead(t), "diff --git a/go.sum")
			out, err := newEngine(t, srv).Review(context.Background(), strings.NewReader(patch), ReviewOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Findings) != 1 || out.Findings[0].StartLine != 44 {
				t.Errorf("findings %+v, want the repaired one", out.Findings)
			}
			reqs := srv.Requests()
			if len(reqs) != 2 {
				t.Fatalf("%d requests, want 2 (answer + repair)", len(reqs))
			}
			repair := reqs[1].Messages[len(reqs[1].Messages)-1].Content
			if !strings.Contains(repair, tt.wantInMsg) {
				t.Errorf("repair prompt %q lacks %q", repair, tt.wantInMsg)
			}
		})
	}
}

func mustRead(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/change.patch")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReview_NothingReviewable(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	patch := "diff --git a/go.sum b/go.sum\n--- a/go.sum\n+++ b/go.sum\n@@ -1 +1,2 @@\n a\n+b\n"
	out, err := newEngine(t, srv).Review(context.Background(), strings.NewReader(patch), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("%d LLM requests for a lockfile-only patch, want 0", len(srv.Requests()))
	}
	if out.Chunks != 0 || len(out.Findings) != 0 || out.Findings == nil || !reflect.DeepEqual(out.Excluded, []string{"go.sum"}) {
		t.Errorf("out %+v", out)
	}
}

func TestReview_SkipOversized(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	e := newEngine(t, srv, func(c *Config) { c.MaxChunkTokens = 40 })

	out, err := e.Review(context.Background(), fixture(t), ReviewOptions{SkipOversized: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.Requests()) != 0 || out.Chunks != 0 {
		t.Errorf("sent %d requests; every hunk exceeds a 40-token budget and should be skipped", len(srv.Requests()))
	}
	if !reflect.DeepEqual(out.Skipped, []string{"internal/store/users.go", "internal/api/handler.go"}) {
		t.Errorf("skipped %v", out.Skipped)
	}

	out, err = e.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Chunks != 2 || len(out.Skipped) != 0 {
		t.Errorf("without SkipOversized: chunks %d skipped %v, want 2 and none", out.Chunks, out.Skipped)
	}
}

func TestReview_ErrorNamesChunkAndStops(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Status: http.StatusUnauthorized}
	})
	_, err := newEngine(t, srv, func(c *Config) { c.MaxChunkTokens = 150; c.Concurrency = 1 }).
		Review(context.Background(), fixture(t), ReviewOptions{})
	if err == nil || !strings.Contains(err.Error(), "chunk 1 of 2") {
		t.Fatalf("err = %v, want it to name chunk 1 of 2", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Errorf("%d requests; a failed chunk should stop the rest", n)
	}
}

func TestReview_RespectsConcurrency(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"s","risk_score":1,"findings":[]}`}
	}).WithDelay(30 * time.Millisecond)

	var patch strings.Builder
	for i := range 8 {
		name := "pkg/f" + string(rune('a'+i)) + ".go"
		patch.WriteString("diff --git a/" + name + " b/" + name + "\n--- a/" + name + "\n+++ b/" + name + "\n@@ -1 +1,2 @@\n x\n+y := computeSomething(argument)\n")
	}
	out, err := newEngine(t, srv, func(c *Config) { c.MaxChunkTokens = 30; c.Concurrency = 3 }).
		Review(context.Background(), strings.NewReader(patch.String()), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Chunks != 8 {
		t.Fatalf("chunks %d, want 8", out.Chunks)
	}
	if got := srv.MaxInFlight(); got > 3 || got < 2 {
		t.Errorf("max in-flight requests %d, want between 2 and 3", got)
	}
}

func TestDescribe(t *testing.T) {
	desc := `{"title":"Build user lookup query inline","type":"Refactor","summary":"s","changes":[
		{"file":"internal/store/users.go","description":"Concatenates the query."},
		{"file":"internal/api/handler.go","description":"Adds a handler."}]}`
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply { return llmtest.Reply{Content: desc} })

	got, err := newEngine(t, srv).Describe(context.Background(), fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != PRTypeRefactor || len(got.Changes) != 2 {
		t.Errorf("got %+v", got)
	}
	prompt := llmtest.UserPrompt(&srv.Requests()[0])
	for _, want := range []string{"### internal/store/users.go (modified)", "### internal/api/handler.go (added)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("describe prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "Only part of the diff") {
		t.Error("prompt claims truncation for a patch that fits")
	}
}

func TestDescribe_TruncatedAndAnchored(t *testing.T) {
	bad := `{"title":"t","type":"Feature","summary":"s","changes":[{"file":"README.md","description":"d"}]}`
	good := `{"title":"t","type":"Feature","summary":"s","changes":[{"file":"internal/api/handler.go","description":"d"}]}`
	srv := llmtest.NewServer(t, func(n int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		if n == 1 {
			return llmtest.Reply{Content: bad}
		}
		return llmtest.Reply{Content: good}
	})
	if _, err := newEngine(t, srv, func(c *Config) { c.MaxChunkTokens = 150 }).Describe(context.Background(), fixture(t)); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if !strings.Contains(llmtest.UserPrompt(&reqs[0]), "Only part of the diff fits below") {
		t.Error("truncated describe prompt does not say so")
	}
	if len(reqs) != 2 || !strings.Contains(reqs[1].Messages[len(reqs[1].Messages)-1].Content, `file "README.md" is not part of this diff`) {
		t.Errorf("want a repair naming README.md; got %d requests", len(reqs))
	}
}

func TestDescribe_NoChanges(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	_, err := newEngine(t, srv).Describe(context.Background(), strings.NewReader(""))
	if !errors.Is(err, ErrNoChanges) {
		t.Errorf("err = %v, want ErrNoChanges", err)
	}
}

func TestImprove(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"suggestions":[
			{"file":"internal/store/users.go","start_line":44,"end_line":45,"category":"Security","summary":"Bind the parameter","existing_code":"q := ...","improved_code":"row := s.db.QueryRowContext(ctx, query, name)"},
			{"file":"internal/api/handler.go","start_line":4,"end_line":4,"category":"Logic","summary":"Handle the error","existing_code":"_ = r.ParseForm()","improved_code":"if err := r.ParseForm(); err != nil { return }"}]}`}
	})
	got, err := newEngine(t, srv).Improve(context.Background(), fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].File != "internal/api/handler.go" || got[1].File != "internal/store/users.go" {
		t.Errorf("suggestions not sorted by file: %+v", got)
	}
}

func TestImprove_InvalidAfterRepairsFails(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"suggestions":[{"file":"internal/api/handler.go","start_line":4,"end_line":4,"category":"Speed","summary":"s","existing_code":"","improved_code":"x"}]}`}
	})
	_, err := newEngine(t, srv, func(c *Config) { c.MaxRepairs = 1 }).Improve(context.Background(), fixture(t))
	if !errors.Is(err, schema.ErrInvalid) {
		t.Errorf("err = %v, want schema.ErrInvalid", err)
	}
}

func TestNew_RequiresProxyAndModel(t *testing.T) {
	if _, err := New(Config{Model: "m"}); err == nil {
		t.Error("want error without BaseURL")
	}
	if _, err := New(Config{BaseURL: "http://x"}); err == nil {
		t.Error("want error without Model")
	}
}

func TestEnginePolicy(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	e := newEngine(t, srv)
	out, err := e.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	def, err := e.Policy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if b := def.Blocking(out.Findings); len(b) != 1 || b[0].Severity != SeverityCritical {
		t.Errorf("default gate blocking %+v, want the Critical finding only", b)
	}
	minor, err := e.Policy([]Severity{SeverityMinor})
	if err != nil {
		t.Fatal(err)
	}
	if b := minor.Blocking(out.Findings); len(b) != 1 || b[0].Severity != SeverityMinor {
		t.Errorf("Minor gate blocking %+v", b)
	}
	if _, err := e.Policy([]Severity{"Blocker"}); !errors.Is(err, schema.ErrInvalid) {
		t.Errorf("unknown severity err = %v", err)
	}
}

func TestReview_InjectsLintOnChangedLinesOnly(t *testing.T) {
	lint, err := os.ReadFile("testdata/golangci.json")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("single chunk", func(t *testing.T) {
		srv := llmtest.NewServer(t, answerByFile)
		out, err := newEngine(t, srv).Review(context.Background(), fixture(t), ReviewOptions{LintReport: bytes.NewReader(lint)})
		if err != nil {
			t.Fatal(err)
		}
		if out.Diagnostics != 2 {
			t.Errorf("Diagnostics = %d, want 2", out.Diagnostics)
		}
		prompt := llmtest.UserPrompt(&srv.Requests()[0])
		block := between(prompt, "<lint>\n", "</lint>")
		want := "internal/api/handler.go:4:13 errcheck: Error return value of `r.ParseForm` is not checked\n" +
			"internal/store/users.go:44:7 gosec: G202: SQL string concatenation\n"
		if block != want {
			t.Errorf("lint block:\n%q\nwant:\n%q", block, want)
		}
		for _, absent := range []string{"revive", "gocyclo", "unused", "internal/other/x.go"} {
			if strings.Contains(prompt, absent) {
				t.Errorf("prompt contains %q, which is not on a changed line", absent)
			}
		}
	})

	t.Run("each chunk gets only its own diagnostics", func(t *testing.T) {
		srv := llmtest.NewServer(t, answerByFile)
		out, err := newEngine(t, srv, func(c *Config) { c.MaxChunkTokens = 150 }).
			Review(context.Background(), fixture(t), ReviewOptions{LintReport: bytes.NewReader(lint)})
		if err != nil {
			t.Fatal(err)
		}
		if out.Chunks != 2 || out.Diagnostics != 2 {
			t.Fatalf("chunks %d diagnostics %d, want 2 and 2", out.Chunks, out.Diagnostics)
		}
		for _, req := range srv.Requests() {
			p := llmtest.UserPrompt(&req)
			users := strings.Contains(p, "### internal/store/users.go")
			if strings.Contains(p, "gosec") != users || strings.Contains(p, "errcheck") == users {
				t.Errorf("chunk with users.go=%v got the wrong diagnostics:\n%s", users, between(p, "<lint>", "</lint>"))
			}
		}
	})

	t.Run("no report, no lint section", func(t *testing.T) {
		srv := llmtest.NewServer(t, answerByFile)
		if _, err := newEngine(t, srv).Review(context.Background(), fixture(t), ReviewOptions{}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(llmtest.UserPrompt(&srv.Requests()[0]), "<lint>") {
			t.Error("lint section rendered without a report")
		}
	})

	t.Run("malformed report fails before any request", func(t *testing.T) {
		srv := llmtest.NewServer(t, answerByFile)
		_, err := newEngine(t, srv).Review(context.Background(), fixture(t), ReviewOptions{LintReport: strings.NewReader("{")})
		if err == nil || !strings.Contains(err.Error(), "golangci-lint JSON") {
			t.Errorf("err = %v", err)
		}
		if len(srv.Requests()) != 0 {
			t.Error("LLM called despite a malformed lint report")
		}
	})
}

func between(s, start, end string) string {
	_, after, ok := strings.Cut(s, start)
	if !ok {
		return ""
	}
	in, _, _ := strings.Cut(after, end)
	return in
}

const docsPatch = `diff --git a/docs/guide.md b/docs/guide.md
--- a/docs/guide.md
+++ b/docs/guide.md
@@ -1,2 +1,3 @@
 # Guide
+Run make test before committing.
 Done.
`

func reviewReply(risk int, severity string) llmtest.Handler {
	findings := "[]"
	if severity != "" {
		findings = `[{"file":"docs/guide.md","start_line":2,"end_line":2,"severity":"` + severity + `","category":"Maintainability","title":"t","body":"b"}]`
	}
	return func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: fmt.Sprintf(`{"summary":"s","risk_score":%d,"findings":%s}`, risk, findings)}
	}
}

func TestReview_RiskAndAutoApprove(t *testing.T) {
	tests := []struct {
		name        string
		patch       string
		handler     llmtest.Handler
		cfg         func(*Config)
		opts        ReviewOptions
		wantApprove bool
	}{
		{"docs only, clean review", docsPatch, reviewReply(3, ""), nil, ReviewOptions{}, true},
		{"docs only, Minor finding", docsPatch, reviewReply(3, "Minor"), nil, ReviewOptions{}, true},
		{"docs only, Major finding", docsPatch, reviewReply(3, "Major"), nil, ReviewOptions{}, false},
		{"docs only, Critical finding", docsPatch, reviewReply(3, "Critical"), nil, ReviewOptions{}, false},
		{"model raises the risk", docsPatch, reviewReply(60, ""), nil, ReviewOptions{}, false},
		{"threshold lowered by policy", docsPatch, reviewReply(3, ""), func(c *Config) { c.RiskPolicy = []byte(`{"auto_approve_threshold":0}`) }, ReviewOptions{}, false},
		{"security-critical diff, clean review", strings.ReplaceAll(mustRead(t), "internal/store/", "internal/auth/"), answerByFileNoFindings, nil, ReviewOptions{}, false},
		{"same diff outside auth", mustRead(t), answerByFileNoFindings, nil, ReviewOptions{}, true},
		{"sensitivity raised by policy", strings.ReplaceAll(mustRead(t), "internal/store/", "internal/auth/"), answerByFileNoFindings, func(c *Config) { c.RiskPolicy = []byte(`{"sensitive_weight": 5}`) }, ReviewOptions{}, true},
		{"only lockfiles", "diff --git a/go.sum b/go.sum\n--- a/go.sum\n+++ b/go.sum\n@@ -1 +1,2 @@\n a\n+b\n", reviewReply(0, ""), nil, ReviewOptions{}, false},
		{"oversized hunk skipped", docsPatch, reviewReply(0, ""), func(c *Config) { c.MaxChunkTokens = 10 }, ReviewOptions{SkipOversized: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := llmtest.NewServer(t, tt.handler)
			var mut []func(*Config)
			if tt.cfg != nil {
				mut = append(mut, tt.cfg)
			}
			out, err := newEngine(t, srv, mut...).Review(context.Background(), strings.NewReader(tt.patch), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if out.AutoApprove != tt.wantApprove {
				t.Errorf("AutoApprove = %v, want %v (risk %d = max(base %d, model %d), findings %d, chunks %d, skipped %v)",
					out.AutoApprove, tt.wantApprove, out.RiskScore, out.BaseRiskScore, out.ModelRiskScore, len(out.Findings), out.Chunks, out.Skipped)
			}
			if out.RiskScore != max(out.BaseRiskScore, out.ModelRiskScore) {
				t.Errorf("RiskScore %d != max(%d, %d)", out.RiskScore, out.BaseRiskScore, out.ModelRiskScore)
			}
		})
	}
}

func answerByFileNoFindings(int, *openai.ChatCompletionRequest) llmtest.Reply {
	return llmtest.Reply{Content: `{"summary":"Looks fine.","risk_score":5,"findings":[]}`}
}

func TestReview_BaseRiskBreakdown(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFileNoFindings)
	auth := strings.ReplaceAll(mustRead(t), "internal/store/", "internal/auth/")
	out, err := newEngine(t, srv).Review(context.Background(), strings.NewReader(auth), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.RiskFiles) != 2 || out.RiskFiles[0].Path != "internal/auth/users.go" || out.RiskFiles[0].Weight != 3 {
		t.Errorf("breakdown %+v, want the auth file first with weight 3", out.RiskFiles)
	}
	if out.ModelRiskScore != 5 || out.BaseRiskScore <= out.ModelRiskScore || out.RiskScore != out.BaseRiskScore {
		t.Errorf("scores: risk %d base %d model %d", out.RiskScore, out.BaseRiskScore, out.ModelRiskScore)
	}
}

func TestNew_RejectsBadRiskPolicy(t *testing.T) {
	srv := llmtest.NewServer(t, answerByFile)
	for name, policy := range map[string]string{
		"schema violation": `{"default_weight": -1}`,
		"malformed glob":   `{"weights": [{"pattern": "[", "weight": 1}]}`,
		"not JSON":         `{`,
	} {
		_, err := New(Config{BaseURL: srv.BaseURL(), Model: "m", RiskPolicy: []byte(policy)})
		if err == nil || !strings.Contains(err.Error(), "risk policy") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
