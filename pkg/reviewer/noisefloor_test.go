package reviewer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
)

// noisyReview answers the fixture's single chunk with two findings above the
// default noise floor and two below it (ISSUE-032).
func noisyReview(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
	p := llmtest.UserPrompt(req)
	if !strings.Contains(p, "### internal/store/users.go") || !strings.Contains(p, "### internal/api/handler.go") {
		return llmtest.Reply{Content: `{"summary":"nothing","risk_score":0,"findings":[]}`}
	}
	return llmtest.Reply{Content: `{"summary":"Four findings.","risk_score":40,"findings":[
		{"file":"internal/store/users.go","start_line":44,"end_line":45,"severity":"Critical","category":"Security","title":"SQL injection","body":"name is concatenated into SQL."},
		{"file":"internal/api/handler.go","start_line":4,"end_line":4,"severity":"Minor","category":"Logic","title":"Form parse error ignored","body":"ParseForm can fail."},
		{"file":"internal/store/users.go","start_line":45,"end_line":45,"severity":"Trivial","category":"Logic","title":"Name the query variable","body":"q is terse."},
		{"file":"internal/store/users.go","start_line":44,"end_line":44,"severity":"Minor","category":"Maintainability","title":"Extract a helper","body":"The query could live in a helper."}]}`}
}

func floorTitles(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for i := range fs {
		out = append(out, fs[i].Title)
	}
	return out
}

func TestReview_NoiseFloorDefault(t *testing.T) {
	e := newEngine(t, llmtest.NewServer(t, noisyReview))
	out, err := e.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(floorTitles(out.Findings), "|"); got != "SQL injection|Form parse error ignored" {
		t.Errorf("findings %q: the Critical and the Minor Logic finding must stay", got)
	}
	if got := strings.Join(floorTitles(out.Filtered), "|"); got != "Extract a helper|Name the query variable" {
		t.Errorf("filtered %q: the Minor Maintainability and the Trivial finding must be filtered, most severe first", got)
	}

	report, err := MarshalReport(out)
	if err != nil {
		t.Fatalf("a report with filtered findings must validate against #ReviewReport: %v", err)
	}
	var back struct {
		Findings []Finding `json:"findings"`
		Filtered []Finding `json:"filtered"`
	}
	if err := json.Unmarshal(report, &back); err != nil || len(back.Findings) != 2 || len(back.Filtered) != 2 {
		t.Errorf("report %s (%v)", report, err)
	}

	md := FormatReview(out)
	if strings.Contains(md, "Extract a helper") || !strings.Contains(md, "2 lower-priority finding(s) below the noise floor") {
		t.Errorf("Markdown must not show filtered findings, only their count:\n%s", md)
	}
}

func TestReview_AllFindingsTurnsTheFloorOff(t *testing.T) {
	e := newEngine(t, llmtest.NewServer(t, noisyReview))
	out, err := e.Review(context.Background(), fixture(t), ReviewOptions{AllFindings: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Findings) != 4 || len(out.Filtered) != 0 {
		t.Errorf("findings %q, filtered %q: AllFindings must keep every finding", floorTitles(out.Findings), floorTitles(out.Filtered))
	}
}

func TestReview_NoiseFloorFromConfig(t *testing.T) {
	srv := llmtest.NewServer(t, noisyReview)
	none := newEngine(t, srv, func(c *Config) { c.NoiseFloor = []byte(`{"rules": []}`) })
	out, err := none.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Findings) != 4 || len(out.Filtered) != 0 {
		t.Errorf("an empty floor must filter nothing: findings %q", floorTitles(out.Findings))
	}

	minor := newEngine(t, srv, func(c *Config) { c.NoiseFloor = []byte(`{"rules": [{"severity": "Minor"}]}`) })
	out, err = minor.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(floorTitles(out.Filtered), "|"); got != "Form parse error ignored|Extract a helper" {
		t.Errorf("a Minor-only floor filtered %q", got)
	}
}

func TestNew_RejectsAnInvalidNoiseFloor(t *testing.T) {
	_, err := New(Config{BaseURL: "http://127.0.0.1:1", APIKey: "k", Model: "m", NoiseFloor: []byte(`{"rules": [{"severity": "Low"}]}`)})
	if err == nil || !strings.Contains(err.Error(), "noise floor") {
		t.Errorf("New with an invalid floor: %v", err)
	}
}

// The floor decides what is shown, never that a human may be skipped:
// auto-approval sees filtered findings too.
func TestReview_NoiseFloorDoesNotUnlockAutoApproval(t *testing.T) {
	trivialOnly := func(_ int, req *openai.ChatCompletionRequest) llmtest.Reply {
		if !strings.Contains(llmtest.UserPrompt(req), "### internal/api/handler.go") {
			return llmtest.Reply{Content: `{"summary":"nothing","risk_score":0,"findings":[]}`}
		}
		return llmtest.Reply{Content: `{"summary":"One nit.","risk_score":95,"findings":[
			{"file":"internal/api/handler.go","start_line":4,"end_line":4,"severity":"Trivial","category":"Maintainability","title":"Rename","body":"r is terse."}]}`}
	}
	e := newEngine(t, llmtest.NewServer(t, trivialOnly))
	out, err := e.Review(context.Background(), fixture(t), ReviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Findings) != 0 || len(out.Filtered) != 1 {
		t.Fatalf("findings %q, filtered %q", floorTitles(out.Findings), floorTitles(out.Filtered))
	}
	if out.AutoApprove {
		t.Error("a high model risk score must still rule out auto-approval when every finding was filtered")
	}
}
