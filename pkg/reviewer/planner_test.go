package reviewer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llmtest"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

var planReq = PlanRequest{
	Issue:    IssuePayload{Title: "Retry transient LiteLLM failures", Body: "429s fail the whole review.", Labels: []string{"enhancement", "llm"}},
	FileTree: []string{"internal/llm/client.go", "internal/llm/client_test.go", "Makefile"},
	OKFIndex: "# /\n\n## Concepts\n\n* [ADR-0004](decisions/adr-0004.md) - transport\n* [Development workflow](development-workflow.md) - loop\n",
}

const goodPlan = `{"summary":"Wrap chat calls in backoff.","affected_files":[
	{"path":"internal/llm/client.go","change":"modify","reason":"add the retry loop"},
	{"path":"internal/llm/retry.go","change":"create","reason":"backoff helper"}],
 "phases":[{"name":"Retry","steps":[{"title":"Add backoff helper","description":"Doubling delay, honors ctx.","files":["internal/llm/retry.go"]},
                                   {"title":"Use it","description":"Retry 429/5xx.","files":["internal/llm/client.go"]}]}],
 "verification":["make test","make lint"],"risks":["retry storms"],"references":["decisions/adr-0004.md"]}`

// The milestone's acceptance test: a plan for a mocked issue payload unifies
// with #ImplementationPlan and comes back typed.
func TestPlan_UnifiesWithSchema(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply { return llmtest.Reply{Content: goodPlan} })
	plan, err := newEngine(t, srv).Plan(context.Background(), &planReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Phases) != 1 || len(plan.Phases[0].Steps) != 2 || plan.Verification[1] != "make lint" || plan.References[0] != "decisions/adr-0004.md" {
		t.Errorf("plan %+v", plan)
	}
	req := srv.Requests()[0]
	user := llmtest.UserPrompt(&req)
	for _, want := range []string{"Issue: Retry transient LiteLLM failures", "Labels: enhancement, llm", "<issue>\n429s fail the whole review.\n</issue>",
		"<tree>\ninternal/llm/client.go\ninternal/llm/client_test.go\nMakefile\n</tree>", "<okf-index>", "decisions/adr-0004.md"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q:\n%s", want, user)
		}
	}
	if !strings.Contains(req.Messages[0].Content, "planning an implementation before any code is written") {
		t.Error("plan did not use the plan template")
	}
}

func TestPlan_AnchorsToTreeAndIndex(t *testing.T) {
	tests := []struct {
		name      string
		bad       string
		req       PlanRequest
		wantInMsg string
	}{
		{"modify a missing file", strings.Replace(goodPlan, `"path":"internal/llm/client.go","change":"modify"`, `"path":"internal/llm/http.go","change":"modify"`, 1), planReq,
			`affected_files.0: "internal/llm/http.go" does not exist in the repository tree, so it cannot be modified`},
		{"create an existing file", strings.Replace(goodPlan, `"path":"internal/llm/retry.go","change":"create"`, `"path":"Makefile","change":"create"`, 1), planReq,
			`affected_files.1: "Makefile" already exists; use modify`},
		{"step file not affected", strings.Replace(goodPlan, `"files":["internal/llm/client.go"]`, `"files":["cmd/pr-review/main.go"]`, 1), planReq,
			`phases.0.steps.1.files.0: "cmd/pr-review/main.go" is not listed in affected_files`},
		{"reference not in index", strings.Replace(goodPlan, `"references":["decisions/adr-0004.md"]`, `"references":["decisions/adr-0099.md"]`, 1), planReq,
			`"decisions/adr-0099.md" is not listed in the knowledge-base index; use one of: decisions/adr-0004.md, development-workflow.md`},
		{"reference without index", goodPlan, PlanRequest{Issue: planReq.Issue, FileTree: planReq.FileTree},
			"no knowledge-base index was provided; omit references"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed := goodPlan
			if tt.req.OKFIndex == "" {
				fixed = strings.Replace(goodPlan, `,"references":["decisions/adr-0004.md"]`, ``, 1)
			}
			srv := llmtest.NewServer(t, func(n int, _ *openai.ChatCompletionRequest) llmtest.Reply {
				if n == 1 {
					return llmtest.Reply{Content: tt.bad}
				}
				return llmtest.Reply{Content: fixed}
			})
			if _, err := newEngine(t, srv).Plan(context.Background(), &tt.req); err != nil {
				t.Fatal(err)
			}
			reqs := srv.Requests()
			if len(reqs) != 2 {
				t.Fatalf("%d requests, want answer + repair", len(reqs))
			}
			if msg := reqs[1].Messages[len(reqs[1].Messages)-1].Content; !strings.Contains(msg, tt.wantInMsg) {
				t.Errorf("repair lacks %q:\n%s", tt.wantInMsg, msg)
			}
		})
	}
}

func TestPlan_WithoutTreeSkipsExistenceChecks(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: strings.Replace(goodPlan, `,"references":["decisions/adr-0004.md"]`, ``, 1)}
	})
	if _, err := newEngine(t, srv).Plan(context.Background(), &PlanRequest{Issue: planReq.Issue}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(llmtest.UserPrompt(&srv.Requests()[0]), "<tree>") {
		t.Error("tree block rendered without a tree")
	}
}

func TestPlan_TreeBudget(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: strings.Replace(goodPlan, `,"references":["decisions/adr-0004.md"]`, ``, 1)}
	})
	tree := make([]string, 0, 3001)
	tree = append(tree, "internal/llm/client.go")
	for i := range 3000 {
		tree = append(tree, fmt.Sprintf("pkg/generated/module_%04d/file_%04d.go", i, i))
	}
	if _, err := newEngine(t, srv).Plan(context.Background(), &PlanRequest{Issue: planReq.Issue, FileTree: tree}); err != nil {
		t.Fatal(err)
	}
	user := llmtest.UserPrompt(&srv.Requests()[0])
	if !strings.Contains(user, "more paths omitted)") || strings.Contains(user, "module_2999") {
		t.Error("oversized tree not truncated with an omitted count")
	}
}

func TestPlan_Errors(t *testing.T) {
	srv := llmtest.NewServer(t, func(int, *openai.ChatCompletionRequest) llmtest.Reply {
		return llmtest.Reply{Content: `{"summary":"x"}`}
	})
	e := newEngine(t, srv, func(c *Config) { c.MaxRepairs = -1 })
	if _, err := e.Plan(context.Background(), &PlanRequest{Issue: IssuePayload{Title: "  "}}); !errors.Is(err, ErrEmptyIssue) {
		t.Errorf("empty issue: %v", err)
	}
	if len(srv.Requests()) != 0 {
		t.Error("request made for an empty issue")
	}
	if _, err := e.Plan(context.Background(), &planReq); !errors.Is(err, schema.ErrInvalid) {
		t.Errorf("incomplete plan: err = %v, want schema.ErrInvalid", err)
	}
}

func TestFormatPlan(t *testing.T) {
	p := &ImplementationPlan{
		Summary:       "Wrap chat calls in backoff.",
		AffectedFiles: []AffectedFile{{Path: "internal/llm/retry.go", Change: "create", Reason: "backoff helper"}},
		Phases:        []PlanPhase{{Name: "Retry", Steps: []PlanStep{{Title: "Add helper", Description: "Doubling delay.", Files: []string{"internal/llm/retry.go"}}}}},
		Verification:  []string{"make test"},
		Risks:         []string{"retry storms"},
		References:    []string{"decisions/adr-0004.md"},
	}
	want := "# Plan: Retry failures\n\nWrap chat calls in backoff.\n" +
		"\n## Affected files\n\n- `internal/llm/retry.go` (create): backoff helper\n" +
		"\n## Phase 1: Retry\n\n- [ ] **Add helper**: Doubling delay. (`internal/llm/retry.go`)\n" +
		"\n## Verification\n\n```sh\nmake test\n```\n" +
		"\n## Risks\n\n- retry storms\n" +
		"\n## References\n\n`decisions/adr-0004.md`\n"
	if got := FormatPlan("Retry failures", p); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
