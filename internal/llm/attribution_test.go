package llm

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

// ISSUE-025: every request carries the context's attribution, and the
// meter sums tokens, cost, retries and repairs across them.
func TestAttributionAndMeter(t *testing.T) {
	srv := llmtest.NewServer(t, func(n int, _ *openai.ChatCompletionRequest) llmtest.Reply {
		switch n {
		case 1:
			return llmtest.Reply{Status: 503}
		case 2:
			return llmtest.Reply{Content: `{"bad":true}`, Cost: "0.0125",
				Usage: openai.Usage{PromptTokens: 1000, CompletionTokens: 200}}
		default:
			return llmtest.Reply{Content: `{"ok":true}`, Cost: "0.0075",
				Usage: openai.Usage{PromptTokens: 1300, CompletionTokens: 100,
					PromptTokensDetails: &openai.PromptTokensDetails{CachedTokens: 900}}}
		}
	})
	c, err := New(Config{BaseURL: srv.BaseURL(), Model: "m", Backoff: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	const trace = "0193a1b2c3d47e5f8a9b0c1d2e3f4a5b"
	m := &Meter{}
	ctx := WithMeter(WithTraceID(WithUser(WithMeta(context.Background(),
		"vizzini.run_id", "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", "vizzini.mode", "review"), "alice"), trace), m)
	ctx = WithMeta(ctx, "vizzini.chunk", "1/2", "vizzini.empty", "")

	if _, err := c.CompleteJSON(ctx, Prompt{System: "s", User: "u"}, func(b []byte) error {
		if string(b) != `{"ok":true}` {
			return errors.New("not ok")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	reqs, hdrs := srv.Requests(), srv.Headers()
	if len(reqs) != 3 {
		t.Fatalf("%d requests, want 3 (503, invalid, valid)", len(reqs))
	}
	want := []struct{ attempt, repair string }{{"0", "0"}, {"1", "0"}, {"0", "1"}}
	tp := regexp.MustCompile(`^00-` + trace + `-([0-9a-f]{16})-01$`)
	spans := map[string]bool{}
	for i, r := range reqs {
		md := r.Metadata
		if md["vizzini.run_id"] != "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b" || md["vizzini.mode"] != "review" || md["vizzini.chunk"] != "1/2" ||
			md["vizzini.attempt"] != want[i].attempt || md["vizzini.repair"] != want[i].repair {
			t.Errorf("request %d metadata %v", i+1, md)
		}
		if _, ok := md["vizzini.empty"]; ok {
			t.Error("empty metadata values must be dropped")
		}
		if r.User != "alice" {
			t.Errorf("request %d user %q", i+1, r.User)
		}
		sm := tp.FindStringSubmatch(hdrs[i].Get("traceparent"))
		if sm == nil {
			t.Fatalf("request %d traceparent %q", i+1, hdrs[i].Get("traceparent"))
		}
		spans[sm[1]] = true
	}
	if len(spans) != 3 {
		t.Error("each request needs its own span id")
	}

	u := m.Usage()
	if u.Requests != 3 || u.Retries != 1 || u.Repairs != 1 || u.InputTokens != 2300 || u.OutputTokens != 300 ||
		u.CachedInputTokens != 900 || u.CostUSD == nil || *u.CostUSD < 0.01999 || *u.CostUSD > 0.02001 {
		t.Errorf("usage %+v (cost %v)", u, u.CostUSD)
	}
}

func TestAttribution_Absent(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{}`}
	})
	c, err := New(Config{BaseURL: srv.BaseURL(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithTraceID(context.Background(), "not-a-trace-id")
	if _, err := c.CompleteJSON(ctx, Prompt{System: "s", User: "u"}, func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if h := srv.Headers()[0].Get("traceparent"); h != "" {
		t.Errorf("invalid trace id must send no traceparent, got %q", h)
	}
	if r := srv.Requests()[0]; r.User != "" || r.Metadata["vizzini.attempt"] != "0" {
		t.Errorf("request %+v", r.Metadata)
	}
	var nilMeter *Meter
	nilMeter.record(func(u *Usage) { u.Requests++ }) // must not panic
}
