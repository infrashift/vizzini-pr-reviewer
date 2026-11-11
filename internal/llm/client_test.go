package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// mockLLM is a scripted OpenAI-compatible chat endpoint.
type mockLLM struct {
	t         *testing.T
	mu        sync.Mutex
	responses []mockResponse
	requests  []openai.ChatCompletionRequest
	headers   []http.Header
}

type mockResponse struct {
	status  int
	content string // assistant message content when status is 200
}

func newMock(t *testing.T, responses ...mockResponse) (*mockLLM, *httptest.Server) {
	t.Helper()
	m := &mockLLM{t: t, responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(srv.Close)
	return m, srv
}

func (m *mockLLM) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Errorf("decode request: %v", err)
	}
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.headers = append(m.headers, r.Header.Clone())
	i := len(m.requests) - 1
	m.mu.Unlock()
	if i >= len(m.responses) {
		m.t.Errorf("unexpected request #%d", i+1)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := m.responses[i]
	w.Header().Set("Content-Type", "application/json")
	if resp.status != http.StatusOK {
		w.WriteHeader(resp.status)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"scripted %d","type":"test"}}`, resp.status)
		return
	}
	_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
		ID: "chatcmpl-test", Object: "chat.completion", Model: req.Model,
		Choices: []openai.ChatCompletionChoice{{
			Message:      openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: resp.content},
			FinishReason: openai.FinishReasonStop,
		}},
	})
}

func (m *mockLLM) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func ok(content string) mockResponse { return mockResponse{status: http.StatusOK, content: content} }
func fail(status int) mockResponse   { return mockResponse{status: status} }

const validReview = `{"summary":"s","risk_score":10,"findings":[]}`

func newClient(t *testing.T, srv *httptest.Server, mutate ...func(*Config)) *Client {
	t.Helper()
	cfg := Config{BaseURL: srv.URL + "/v1", APIKey: "sk-test", Model: "test-model", Backoff: time.Millisecond}
	for _, f := range mutate {
		f(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func reviewValidator(t *testing.T) (func([]byte) error, *schema.ReviewOutput) {
	t.Helper()
	v, err := schema.New()
	if err != nil {
		t.Fatal(err)
	}
	out := &schema.ReviewOutput{}
	return func(b []byte) error {
		r, err := v.ValidateReview(b)
		if err == nil {
			*out = *r
		}
		return err
	}, out
}

var testPrompt = Prompt{System: "sys", User: "usr"}

func TestCompleteJSON_Success(t *testing.T) {
	m, srv := newMock(t, ok(validReview))
	validate, out := reviewValidator(t)

	raw, err := newClient(t, srv).CompleteJSON(context.Background(), testPrompt, validate)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != validReview || out.RiskScore != 10 {
		t.Errorf("raw %s, decoded %+v", raw, out)
	}

	req := m.requests[0]
	if req.Model != "test-model" {
		t.Errorf("model %q", req.Model)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
		t.Errorf("response_format %+v, want json_object", req.ResponseFormat)
	}
	if len(req.Messages) != 2 || req.Messages[0].Content != "sys" || req.Messages[1].Content != "usr" {
		t.Errorf("messages %+v", req.Messages)
	}
	if got := m.headers[0].Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization %q", got)
	}
}

func TestCompleteJSON_RetriesTransientErrors(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusTooManyRequests, http.StatusRequestTimeout} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			m, srv := newMock(t, fail(status), fail(status), ok(validReview))
			validate, _ := reviewValidator(t)
			if _, err := newClient(t, srv).CompleteJSON(context.Background(), testPrompt, validate); err != nil {
				t.Fatal(err)
			}
			if m.count() != 3 {
				t.Errorf("%d requests, want 3", m.count())
			}
		})
	}
}

func TestCompleteJSON_GivesUpAfterMaxRetries(t *testing.T) {
	m, srv := newMock(t, fail(500), fail(500), fail(500))
	validate, _ := reviewValidator(t)
	_, err := newClient(t, srv, func(c *Config) { c.MaxRetries = 2 }).CompleteJSON(context.Background(), testPrompt, validate)
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != 500 {
		t.Errorf("error %v does not wrap the 500 APIError", err)
	}
	if m.count() != 3 {
		t.Errorf("%d requests, want 3 (1 + 2 retries)", m.count())
	}
}

func TestCompleteJSON_DoesNotRetryClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			m, srv := newMock(t, fail(status))
			validate, _ := reviewValidator(t)
			if _, err := newClient(t, srv).CompleteJSON(context.Background(), testPrompt, validate); err == nil {
				t.Fatal("want error")
			}
			if m.count() != 1 {
				t.Errorf("%d requests, want 1", m.count())
			}
		})
	}
}

func TestCompleteJSON_RepairsInvalidAnswer(t *testing.T) {
	bad := `{"summary":"s","risk_score":10,"findings":[{"file":"a.go","start_line":1,"end_line":1,"severity":"High","category":"Logic","title":"t","body":"b"}]}`
	good := strings.Replace(bad, `"High"`, `"Major"`, 1)
	m, srv := newMock(t, ok(bad), ok(good))
	validate, out := reviewValidator(t)

	if _, err := newClient(t, srv).CompleteJSON(context.Background(), testPrompt, validate); err != nil {
		t.Fatal(err)
	}
	if len(out.Findings) != 1 || out.Findings[0].Severity != schema.SeverityMajor {
		t.Errorf("decoded %+v", out)
	}

	second := m.requests[1].Messages
	if len(second) != 4 {
		t.Fatalf("repair request has %d messages, want 4 (system, user, assistant, repair)", len(second))
	}
	if second[2].Role != openai.ChatMessageRoleAssistant || second[2].Content != bad {
		t.Errorf("assistant echo %+v", second[2])
	}
	if second[3].Role != openai.ChatMessageRoleUser || !strings.Contains(second[3].Content, "severity") {
		t.Errorf("repair message does not cite the validation error: %q", second[3].Content)
	}
}

func TestCompleteJSON_GivesUpAfterMaxRepairs(t *testing.T) {
	m, srv := newMock(t, ok(`{"nope":1}`), ok(`{"nope":2}`))
	validate, _ := reviewValidator(t)
	_, err := newClient(t, srv, func(c *Config) { c.MaxRepairs = 1 }).CompleteJSON(context.Background(), testPrompt, validate)
	if !errors.Is(err, schema.ErrInvalid) {
		t.Fatalf("error %v does not wrap schema.ErrInvalid", err)
	}
	if m.count() != 2 {
		t.Errorf("%d requests, want 2", m.count())
	}
}

func TestCompleteJSON_StripsCodeFence(t *testing.T) {
	_, srv := newMock(t, ok("```json\n"+validReview+"\n```"))
	validate, _ := reviewValidator(t)
	raw, err := newClient(t, srv).CompleteJSON(context.Background(), testPrompt, validate)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != validReview {
		t.Errorf("raw %q", raw)
	}
}

func TestCompleteJSON_ContextCancelledDuringBackoff(t *testing.T) {
	_, srv := newMock(t, fail(503), ok(validReview))
	validate, _ := reviewValidator(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := newClient(t, srv, func(c *Config) { c.Backoff = time.Hour }).CompleteJSON(ctx, testPrompt, validate)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v, want deadline exceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("backoff ignored context cancellation")
	}
}

func TestNew_RequiresBaseURLAndModel(t *testing.T) {
	if _, err := New(Config{Model: "m"}); err == nil {
		t.Error("want error without BaseURL")
	}
	if _, err := New(Config{BaseURL: "http://x"}); err == nil {
		t.Error("want error without Model")
	}
}

func TestRenderReview(t *testing.T) {
	p, err := Render("review", ReviewData{Diff: "### a.go (modified)\n@@ -1 +1 @@\n1 1   x\n", Part: 2, Parts: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schema.Severities {
		if !strings.Contains(p.System, `"`+string(s)+`"`) {
			t.Errorf("system prompt lacks severity %q", s)
		}
	}
	for _, c := range schema.Categories {
		if !strings.Contains(p.System, `"`+string(c)+`"`) {
			t.Errorf("system prompt lacks category %q", c)
		}
	}
	for _, field := range []string{"summary", "risk_score", "findings", "start_line", "end_line", "suggestion"} {
		if !strings.Contains(p.System, `"`+field+`"`) {
			t.Errorf("system prompt lacks field %q", field)
		}
	}
	if !strings.Contains(p.User, "### a.go (modified)") || !strings.Contains(p.User, "part 2 of 3") {
		t.Errorf("user prompt:\n%s", p.User)
	}

	single, err := Render("review", ReviewData{Diff: "d", Part: 1, Parts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(single.User, "part 1 of 1") {
		t.Error("single-chunk prompt mentions parts")
	}
}

func TestRender_Errors(t *testing.T) {
	if _, err := Render("nope", nil); err == nil {
		t.Error("want error for unknown template")
	}
	if _, err := Render("review", map[string]any{"Diff": "d"}); err == nil {
		t.Error("want error for missing template data (missingkey=error)")
	}
}
