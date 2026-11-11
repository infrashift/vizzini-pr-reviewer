package llm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"

	openai "github.com/sashabaranov/go-openai"
)

// Request-scoped attribution (ISSUE-025, ADR-0015). Everything here travels
// on the context, so every request a call makes carries it without changing
// any signature: metadata for LiteLLM's spend logs, the human as the
// OpenAI "user" (LiteLLM's end user), a W3C traceparent, and a Meter that
// sums what the requests cost.

type ctxKey int

const (
	metaKey ctxKey = iota
	userKey
	traceKey
	meterKey
)

// WithMeta returns ctx with key/value pairs added to the metadata every
// request sends. Later values win. Odd trailing keys are ignored.
func WithMeta(ctx context.Context, kv ...string) context.Context {
	m := maps.Clone(metaFrom(ctx))
	if m == nil {
		m = map[string]string{}
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			m[kv[i]] = kv[i+1]
		}
	}
	return context.WithValue(ctx, metaKey, m)
}

func metaFrom(ctx context.Context) map[string]string {
	m, _ := ctx.Value(metaKey).(map[string]string)
	return m
}

// WithUser sets the human the requests are made for (LiteLLM's end user).
func WithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// WithTraceID sets the W3C trace ID (32 lower-case hex digits) sent in a
// traceparent header on every request; each request gets its own span ID.
// An invalid ID is ignored.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if len(traceID) != 32 || strings.Trim(traceID, "0123456789abcdef") != "" || strings.Trim(traceID, "0") == "" {
		return ctx
	}
	return context.WithValue(ctx, traceKey, traceID)
}

// WithMeter makes every request record its usage in m.
func WithMeter(ctx context.Context, m *Meter) context.Context {
	return context.WithValue(ctx, meterKey, m)
}

func meterFrom(ctx context.Context) *Meter {
	m, _ := ctx.Value(meterKey).(*Meter)
	return m
}

// Usage is what a set of requests consumed.
type Usage struct {
	// Requests counts HTTP attempts, including failed ones.
	Requests int `json:"requests"`
	// Retries counts attempts repeated after a transient failure.
	Retries int `json:"retries"`
	// Repairs counts answers sent back to the model for a schema or
	// anchoring fix: tokens spent only because an answer was invalid.
	Repairs           int `json:"repairs"`
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	// CostUSD sums the proxy's per-response cost (LiteLLM's
	// x-litellm-response-cost header); nil when no response reported one.
	CostUSD *float64 `json:"cost_usd,omitempty"`
}

// Meter accumulates Usage across concurrent requests.
type Meter struct {
	mu sync.Mutex
	u  Usage
}

// Usage returns a copy of what has been recorded.
func (m *Meter) Usage() Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.u
	if u.CostUSD != nil {
		c := *u.CostUSD
		u.CostUSD = &c
	}
	return u
}

func (m *Meter) record(f func(u *Usage)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f(&m.u)
}

func (m *Meter) response(resp *openai.ChatCompletionResponse) {
	m.record(func(u *Usage) {
		u.InputTokens += resp.Usage.PromptTokens
		u.OutputTokens += resp.Usage.CompletionTokens
		if d := resp.Usage.PromptTokensDetails; d != nil {
			u.CachedInputTokens += d.CachedTokens
		}
		if c, err := strconv.ParseFloat(resp.Header().Get("x-litellm-response-cost"), 64); err == nil && c >= 0 {
			if u.CostUSD == nil {
				u.CostUSD = new(float64)
			}
			*u.CostUSD += c
		}
	})
}

// attemptContext adds one attempt's numbers to ctx's metadata and fills the
// request's body attribution (metadata and user) from it. The returned
// context must be the one the request is sent with: the transport reads the
// same metadata for LiteLLM's headers.
func attemptContext(ctx context.Context, req *openai.ChatCompletionRequest, repair, attempt int) context.Context {
	ctx = WithMeta(ctx, "vizzini.repair", strconv.Itoa(repair), "vizzini.attempt", strconv.Itoa(attempt))
	req.Metadata = maps.Clone(metaFrom(ctx))
	if u, _ := ctx.Value(userKey).(string); u != "" {
		req.User = u
	}
	return ctx
}

// traceTransport adds attribution headers from the request's context: a W3C
// traceparent when it carries a trace ID, and LiteLLM's
// x-litellm-spend-logs-metadata (the metadata as JSON; LiteLLM keeps only
// this in its spend logs, not the body "metadata") and x-litellm-tags
// ("vizzini" and "vizzini:<mode>").
type traceTransport struct{ next http.RoundTripper }

func (t traceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	id, _ := r.Context().Value(traceKey).(string)
	md := metaFrom(r.Context())
	if id == "" && len(md) == 0 {
		return t.next.RoundTrip(r)
	}
	r = r.Clone(r.Context())
	if id != "" {
		span := make([]byte, 8)
		_, _ = rand.Read(span)
		r.Header.Set("traceparent", "00-"+id+"-"+hex.EncodeToString(span)+"-01")
	}
	if len(md) > 0 {
		if b, err := json.Marshal(md); err == nil {
			r.Header.Set("x-litellm-spend-logs-metadata", string(b))
		}
		tags := "vizzini"
		if mode := md["vizzini.mode"]; mode != "" {
			tags += ",vizzini:" + mode
		}
		r.Header.Set("x-litellm-tags", tags)
	}
	return t.next.RoundTrip(r)
}

// withTrace returns a copy of c whose transport adds traceparent headers.
func withTrace(c *http.Client) *http.Client {
	cp := *c
	next := cp.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	cp.Transport = traceTransport{next: next}
	return &cp
}
