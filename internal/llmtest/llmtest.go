// Package llmtest provides test helpers: a scripted OpenAI-compatible
// endpoint standing in for the LiteLLM proxy, and offline tokenizer setup.
package llmtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Reply is the mock's answer to one chat request. A zero Status means 200.
type Reply struct {
	Status  int
	Content string
	// Usage is reported in the response body, as LiteLLM does.
	Usage openai.Usage
	// Cost, if set, is sent as LiteLLM's x-litellm-response-cost header.
	Cost string
}

// Handler decides the reply for a request. n is the 1-based request count.
type Handler func(n int, req *openai.ChatCompletionRequest) Reply

// Server is a mock /v1/chat/completions endpoint.
type Server struct {
	*httptest.Server
	t       testing.TB
	handler Handler
	delay   time.Duration

	mu       sync.Mutex
	requests []openai.ChatCompletionRequest
	headers  []http.Header

	inFlight    atomic.Int32
	maxInFlight atomic.Int32
}

// NewServer starts a mock that answers with handler; it is closed with the test.
func NewServer(t testing.TB, handler Handler) *Server {
	t.Helper()
	s := &Server{t: t, handler: handler}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// WithDelay makes every reply wait d, to observe concurrency.
func (s *Server) WithDelay(d time.Duration) *Server {
	s.delay = d
	return s
}

// BaseURL is the API root to configure clients with.
func (s *Server) BaseURL() string { return s.URL + "/v1" }

// Requests returns a copy of the requests received so far.
func (s *Server) Requests() []openai.ChatCompletionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]openai.ChatCompletionRequest(nil), s.requests...)
}

// Headers returns a copy of each request's HTTP headers, in arrival order.
func (s *Server) Headers() []http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]http.Header, len(s.headers))
	for i, h := range s.headers {
		out[i] = h.Clone()
	}
	return out
}

// MaxInFlight is the highest number of concurrent requests observed.
func (s *Server) MaxInFlight() int { return int(s.maxInFlight.Load()) }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	cur := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		prev := s.maxInFlight.Load()
		if cur <= prev || s.maxInFlight.CompareAndSwap(prev, cur) {
			break
		}
	}

	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.t.Errorf("llmtest: decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.headers = append(s.headers, r.Header.Clone())
	n := len(s.requests)
	s.mu.Unlock()

	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	reply := s.handler(n, &req)
	w.Header().Set("Content-Type", "application/json")
	if reply.Cost != "" {
		w.Header().Set("x-litellm-response-cost", reply.Cost)
	}
	if reply.Status != 0 && reply.Status != http.StatusOK {
		w.WriteHeader(reply.Status)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"llmtest scripted %d","type":"test"}}`, reply.Status)
		return
	}
	_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
		ID: fmt.Sprintf("chatcmpl-%d", n), Object: "chat.completion", Model: req.Model,
		Choices: []openai.ChatCompletionChoice{{
			Message:      openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: reply.Content},
			FinishReason: openai.FinishReasonStop,
		}},
		Usage: reply.Usage,
	})
}

// UserPrompt returns the content of the first user message.
func UserPrompt(req *openai.ChatCompletionRequest) string {
	for i := range req.Messages {
		if req.Messages[i].Role == openai.ChatMessageRoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}

// Offline prepares a test binary to count tokens without the network: it
// points TIKTOKEN_CACHE_DIR at the repository's seeded cache (unless already
// set) and routes outbound HTTP(S) to a dead proxy. Loopback servers such as
// httptest are never proxied, so mocks keep working. Call it from TestMain.
func Offline() error {
	if os.Getenv("TIKTOKEN_CACHE_DIR") == "" {
		root, err := moduleRoot()
		if err != nil {
			return err
		}
		if err := os.Setenv("TIKTOKEN_CACHE_DIR", filepath.Join(root, ".cache", "tiktoken")); err != nil {
			return err
		}
	}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if err := os.Setenv(k, "http://127.0.0.1:1"); err != nil {
			return err
		}
	}
	for _, k := range []string{"NO_PROXY", "no_proxy"} {
		if err := os.Unsetenv(k); err != nil {
			return err
		}
	}
	return nil
}

// RunOffline is a TestMain body: Offline, then m.Run.
func RunOffline(m *testing.M) {
	if err := Offline(); err != nil {
		fmt.Fprintln(os.Stderr, "llmtest:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above the working directory")
		}
		dir = parent
	}
}
