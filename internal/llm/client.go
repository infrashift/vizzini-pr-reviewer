// Package llm renders embedded prompt templates and calls an OpenAI-compatible
// endpoint (an internal LiteLLM proxy, or a mock in tests) in JSON mode, with
// retry backoff and schema-driven repair (ADR-0004).
package llm

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("").
	Funcs(template.FuncMap{
		"severities": func() string { return quoteJoin(schema.Severities) },
		"categories": func() string { return quoteJoin(schema.Categories) },
		"prtypes":    func() string { return quoteJoin(schema.PRTypes) },
	}).
	Option("missingkey=error").
	ParseFS(templateFS, "templates/*.tmpl"))

func quoteJoin[T ~string](vals []T) string {
	q := make([]string, 0, len(vals))
	for _, v := range vals {
		q = append(q, `"`+string(v)+`"`)
	}
	return strings.Join(q, ", ")
}

// Prompt is a rendered system/user message pair.
type Prompt struct {
	System string
	User   string
}

// ReviewData feeds the "review" template.
type ReviewData struct {
	// Diff is one rendered chunk (see chunker.Chunk.Render).
	Diff string
	// Part and Parts number the chunk within the whole patch, starting at 1.
	Part, Parts int
	// Lint holds linter diagnostics on this chunk's changed lines, one
	// "file:line:col linter: text" entry each (Milestone 4.2).
	Lint []string
	// Knowledge holds the OKF documents relevant to this chunk (Milestone 4.5).
	Knowledge []KnowledgeDoc
	// Context holds read-only code from outside the diff: definitions of
	// symbols the chunk uses and callers of symbols it changes (ISSUE-023).
	Context []ContextSnippet
}

// ContextSnippet is one region of a repository file given as context.
type ContextSnippet struct {
	Path       string
	Start, End int
	// Kind is "definition" or "caller"; Symbol is what it was found for.
	Kind, Symbol string
	// Text is the region with each line prefixed by its line number.
	Text string
}

// KnowledgeDoc is one knowledge-base document given to the model.
type KnowledgeDoc struct {
	ID, Title, Body string
}

// DescribeData feeds the "describe" template.
type DescribeData struct {
	// Files holds one header line per changed file, including files whose
	// content did not fit (see Truncated).
	Files []string
	// Diff is the rendered content that fit the token budget.
	Diff string
	// Truncated is true when some file content was left out of Diff.
	Truncated bool
}

// ImproveData feeds the "improve" template.
type ImproveData = ReviewData

// CommandData feeds the "explain", "refactor" and "docstrings" templates.
type CommandData struct {
	// Target is "file:start-end" (or just "file" for docstrings).
	Target string
	// Question is free text after the target, if any.
	Question string
	// Diff holds only the hunks the command is scoped to.
	Diff string
	// Thread is the review-thread history, oldest first.
	Thread []ThreadMessage
}

// PlanData feeds the "plan" template.
type PlanData struct {
	Title    string
	Body     string
	Labels   []string
	FileTree string
	// TreeOmitted counts paths left out of FileTree for budget.
	TreeOmitted int
	OKFIndex    string
}

// ThreadMessage is one comment in a review thread.
type ThreadMessage struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

// Render executes the "<name>.system" and "<name>.user" templates.
func Render(name string, data any) (Prompt, error) {
	var sys, usr strings.Builder
	if err := templates.ExecuteTemplate(&sys, name+".system", data); err != nil {
		return Prompt{}, fmt.Errorf("render %s.system: %w", name, err)
	}
	if err := templates.ExecuteTemplate(&usr, name+".user", data); err != nil {
		return Prompt{}, fmt.Errorf("render %s.user: %w", name, err)
	}
	return Prompt{System: strings.TrimSpace(sys.String()), User: strings.TrimSpace(usr.String())}, nil
}

// Config configures a Client. For the int and duration fields, zero selects
// the default and a negative value disables the feature.
type Config struct {
	// BaseURL is the OpenAI-compatible API root, e.g. "http://litellm:4000/v1".
	BaseURL string
	APIKey  string
	Model   string
	// MaxRetries bounds retries of transient failures (network errors, 408,
	// 429, 5xx). Default 3.
	MaxRetries int
	// MaxRepairs bounds how often an invalid answer is sent back to the model
	// with the validation error. Default 2.
	MaxRepairs int
	// Backoff is the first retry delay; it doubles on each retry. Default 500ms.
	Backoff time.Duration
	// HTTPClient overrides the transport. Default: http.Client with a 5 minute timeout.
	HTTPClient *http.Client
}

const (
	defaultMaxRetries = 3
	defaultMaxRepairs = 2
	defaultBackoff    = 500 * time.Millisecond
	defaultTimeout    = 5 * time.Minute
)

// Client calls the LLM. It is safe for concurrent use.
type Client struct {
	api        *openai.Client
	model      string
	maxRetries int
	maxRepairs int
	backoff    time.Duration
}

// New returns a Client for cfg.
func New(cfg Config) (*Client, error) { //nolint:gocritic // hugeParam: one-shot constructor; by-value Config keeps call sites simple

	if cfg.BaseURL == "" {
		return nil, errors.New("llm: BaseURL is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("llm: Model is required")
	}
	oc := openai.DefaultConfig(cfg.APIKey)
	oc.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTPClient != nil {
		oc.HTTPClient = withTrace(cfg.HTTPClient)
	} else {
		oc.HTTPClient = withTrace(&http.Client{Timeout: defaultTimeout})
	}
	return &Client{
		api:        openai.NewClientWithConfig(oc),
		model:      cfg.Model,
		maxRetries: orDefault(cfg.MaxRetries, defaultMaxRetries),
		maxRepairs: orDefault(cfg.MaxRepairs, defaultMaxRepairs),
		backoff:    orDefault(cfg.Backoff, defaultBackoff),
	}, nil
}

func orDefault[T int | time.Duration](v, def T) T {
	switch {
	case v == 0:
		return def
	case v < 0:
		return 0
	default:
		return v
	}
}

// CompleteJSON sends p in JSON mode and passes the answer to validate. When
// validate rejects it, the error is sent back to the model for a corrected
// answer, up to MaxRepairs times. It returns the first answer validate
// accepts; callers keep the decoded value from inside validate.
func (c *Client) CompleteJSON(ctx context.Context, p Prompt, validate func([]byte) error) ([]byte, error) {
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: p.System},
		{Role: openai.ChatMessageRoleUser, Content: p.User},
	}
	for repair := 0; ; repair++ {
		if repair > 0 {
			meterFrom(ctx).record(func(u *Usage) { u.Repairs++ })
		}
		content, err := c.chat(ctx, msgs, repair)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(content)
		verr := validate(raw)
		if verr == nil {
			return raw, nil
		}
		if repair >= c.maxRepairs {
			return nil, fmt.Errorf("llm: answer still invalid after %d repair attempt(s): %w", repair, verr)
		}
		msgs = append(msgs,
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: content},
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: repairPrompt(verr)},
		)
	}
}

func repairPrompt(err error) string {
	return "Your previous answer was rejected by the schema validator:\n" + err.Error() +
		"\n\nReturn the corrected JSON object only, with no prose and no code fences."
}

// chat performs one completion, retrying transient failures with backoff.
func (c *Client) chat(ctx context.Context, msgs []openai.ChatCompletionMessage, repair int) (string, error) {
	req := openai.ChatCompletionRequest{
		Model:          c.model,
		Messages:       msgs,
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
	}
	meter := meterFrom(ctx)
	delay := c.backoff
	for attempt := 0; ; attempt++ {
		actx := attemptContext(ctx, &req, repair, attempt)
		meter.record(func(u *Usage) {
			u.Requests++
			if attempt > 0 {
				u.Retries++
			}
		})
		resp, err := c.api.CreateChatCompletion(actx, req)
		if err == nil {
			meter.response(&resp)
			if len(resp.Choices) == 0 {
				return "", errors.New("llm: response has no choices")
			}
			return resp.Choices[0].Message.Content, nil
		}
		if !retryable(ctx, err) || attempt >= c.maxRetries {
			return "", fmt.Errorf("llm: chat completion (attempt %d): %w", attempt+1, err)
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return "", fmt.Errorf("llm: chat completion: %w", ctx.Err())
		case <-t.C:
		}
		delay *= 2
	}
}

// retryable reports whether err is transient: a network failure, a timeout,
// rate limiting or a server error. Client errors (4xx) and cancellation are not.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var apiErr *openai.APIError
	var reqErr *openai.RequestError
	switch {
	case errors.As(err, &apiErr):
		return retryableStatus(apiErr.HTTPStatusCode)
	case errors.As(err, &reqErr):
		return retryableStatus(reqErr.HTTPStatusCode)
	default:
		return true // transport-level failure
	}
}

func retryableStatus(status int) bool {
	return status == 0 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

// extractJSON tolerates models that wrap JSON-mode output in a Markdown code
// fence despite instructions.
func extractJSON(content string) []byte {
	s := strings.TrimSpace(content)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
	}
	return []byte(s)
}
