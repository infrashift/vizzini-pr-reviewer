package reviewer

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llm"
)

// Attribution says who and what a run is for, so its cost can be attributed
// per request, per run and across a pull request's lifecycle (ISSUE-025,
// ADR-0015). It travels on the context: set it once with WithAttribution
// and every request of Review, Describe, Improve, Plan and RunCommand
// carries it to the LiteLLM proxy as request metadata, which LiteLLM keeps
// with each spend-log row.
type Attribution struct {
	// RunID identifies one invocation. Empty means Vizzini generates a
	// UUIDv7. It doubles as the W3C trace ID of every request.
	RunID string `json:"run_id,omitempty"`
	// PRKey identifies the pull request across its lifecycle, e.g.
	// "ado:dev.azure.com/org/project/repo#42".
	PRKey string `json:"pr_key,omitempty"`
	// Iteration is the head commit the run reviewed.
	Iteration string `json:"iteration,omitempty"`
	// Requester is the human the run is for (forge user, CI actor). It is
	// sent as the request's user, LiteLLM's end user. It is self-reported:
	// use per-team virtual keys for enforceable chargeback.
	Requester string `json:"requester,omitempty"`
	// Agent is the tool that asked, e.g. "mrman/v0.1.1" or "pr-review".
	Agent string `json:"agent,omitempty"`
}

// Usage is what a run's model requests consumed: request, retry and repair
// counts, input, output and cached-input tokens, and the proxy-reported
// cost in US dollars when available.
type Usage = llm.Usage

type attributionKey struct{}

// RunInfo is one engine call: its attribution (RunID always set), the
// mode, and what its requests consumed.
type RunInfo struct {
	Attribution
	Mode  string `json:"mode"`
	Usage Usage  `json:"usage"`
}

// RunRecorder collects a RunInfo from every engine call made with a
// context carrying it (WithRunRecorder), so callers get usage from modes
// whose results have no room for it: Describe, Improve, Plan and
// RunCommand. Safe for concurrent use.
type RunRecorder struct {
	mu   sync.Mutex
	runs []RunInfo
}

// Runs returns the recorded runs, oldest first.
func (r *RunRecorder) Runs() []RunInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.runs)
}

type recorderKey struct{}

// WithRunRecorder returns ctx with r collecting every engine call's run.
func WithRunRecorder(ctx context.Context, r *RunRecorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, r)
}

// WithAttribution returns ctx carrying a for every request made with it.
func WithAttribution(ctx context.Context, a Attribution) context.Context { //nolint:gocritic // hugeParam: a context value is copied by design
	return context.WithValue(ctx, attributionKey{}, a)
}

// AttributionFrom returns the attribution on ctx, if any.
func AttributionFrom(ctx context.Context) Attribution {
	a, _ := ctx.Value(attributionKey{}).(Attribution)
	return a
}

// NewRunID returns a UUIDv7: a 48-bit Unix-millisecond timestamp, so IDs
// sort by creation time, followed by random bits.
func NewRunID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	ms := uint64(time.Now().UnixMilli())
	binary.BigEndian.PutUint16(b[0:2], uint16(ms>>32)) //nolint:gosec // G115: deliberate truncation to the UUIDv7 field
	binary.BigEndian.PutUint32(b[2:6], uint32(ms))     //nolint:gosec // G115: deliberate truncation to the UUIDv7 field
	b[6] = 0x70 | b[6]&0x0f                            // version 7
	b[8] = 0x80 | b[8]&0x3f                            // RFC 9562 variant
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// begin prepares ctx for one mode's requests: the attribution (with a run
// ID generated if missing) as request metadata, the requester as the user,
// the run ID as the trace ID, and a fresh meter. Call finish when the mode
// returns: it hands the run to the context's RunRecorder, if any.
func begin(ctx context.Context, mode string) (_ context.Context, _ Attribution, _ *llm.Meter, finish func()) {
	a := AttributionFrom(ctx)
	if a.RunID == "" {
		a.RunID = NewRunID()
	}
	m := &llm.Meter{}
	ctx = llm.WithMeta(ctx,
		"vizzini.run_id", a.RunID,
		"vizzini.pr_key", a.PRKey,
		"vizzini.iteration", a.Iteration,
		"vizzini.requester", a.Requester,
		"vizzini.agent", a.Agent,
		"vizzini.mode", mode,
	)
	ctx = llm.WithUser(ctx, a.Requester)
	ctx = llm.WithTraceID(ctx, strings.ReplaceAll(strings.ToLower(a.RunID), "-", ""))
	rec, _ := ctx.Value(recorderKey{}).(*RunRecorder)
	finish = func() {
		if rec == nil {
			return
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.runs = append(rec.runs, RunInfo{Attribution: a, Mode: mode, Usage: m.Usage()})
	}
	return llm.WithMeter(ctx, m), a, m, finish
}
