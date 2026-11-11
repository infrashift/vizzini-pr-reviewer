package reviewer

import (
	"io"
	"net/http"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// Severity ranks a finding: Critical, Major, Minor or Trivial.
type Severity = schema.Severity

// Severities, most to least severe.
const (
	SeverityCritical = schema.SeverityCritical
	SeverityMajor    = schema.SeverityMajor
	SeverityMinor    = schema.SeverityMinor
	SeverityTrivial  = schema.SeverityTrivial
)

// Category classifies a finding or suggestion.
type Category = schema.Category

// Categories.
const (
	CategorySecurity        = schema.CategorySecurity
	CategoryPerformance     = schema.CategoryPerformance
	CategoryLogic           = schema.CategoryLogic
	CategoryMaintainability = schema.CategoryMaintainability
)

// PRType classifies a pull request.
type PRType = schema.PRType

// PR types.
const (
	PRTypeFeature  = schema.PRTypeFeature
	PRTypeBugfix   = schema.PRTypeBugfix
	PRTypeRefactor = schema.PRTypeRefactor
	PRTypeDocs     = schema.PRTypeDocs
	PRTypeTests    = schema.PRTypeTests
	PRTypeChore    = schema.PRTypeChore
)

// PolicyGate lists the severities that block a change (Milestone 4.1).
// Build one with Engine.Policy so it is validated by the schema.
type PolicyGate = schema.PolicyGate

// Finding is one review comment anchored to new-file lines.
type Finding = schema.Finding

// FileChange is one line of a PR description's change list.
type FileChange = schema.FileChange

// PRDescription is the result of Describe.
type PRDescription = schema.PRDescription

// CodeSuggestion is one concrete replacement proposed by Improve.
type CodeSuggestion = schema.CodeSuggestion

// ReviewOutput is the merged result of Review across all chunks.
type ReviewOutput struct {
	// Summary is the model's summary. With several chunks it holds one
	// paragraph per chunk, prefixed "Part i of n:".
	Summary string `json:"summary"`
	// RiskScore is the highest risk score any chunk reported (0–100).
	RiskScore int `json:"risk_score"`
	// Findings are sorted by severity (most severe first), then file and line.
	Findings []Finding `json:"findings"`
	// Excluded lists generated or vendored paths that were not reviewed.
	Excluded []string `json:"excluded,omitempty"`
	// Skipped lists files with a hunk too large for the token budget that
	// were left out because ReviewOptions.SkipOversized was set.
	Skipped []string `json:"skipped,omitempty"`
	// Chunks is the number of LLM requests the review took.
	Chunks int `json:"chunks"`
	// Diagnostics counts the linter diagnostics on changed lines that were
	// added to the prompts (see ReviewOptions.LintReport).
	Diagnostics int `json:"diagnostics,omitempty"`
}

// ReviewOptions tunes a single Review call.
type ReviewOptions struct {
	// SkipOversized leaves out any hunk larger than the token budget instead
	// of sending it in a chunk of its own. Skipped files are reported in
	// ReviewOutput.Skipped.
	SkipOversized bool
	// LintReport, if set, is golangci-lint JSON output (v1 --out-format=json
	// or v2 --output.json.path=stdout) produced from the repository root.
	// Diagnostics on lines the diff adds are given to the model so it can
	// explain them and propose fixes; all others are ignored.
	LintReport io.Reader
}

// Config configures an Engine. Zero values select the defaults.
type Config struct {
	// BaseURL is the OpenAI-compatible API root of the LiteLLM proxy,
	// e.g. "http://litellm:4000/v1". Required.
	BaseURL string
	// APIKey is sent as a bearer token.
	APIKey string
	// Model is the model name as configured in LiteLLM. Required.
	Model string

	// MaxRetries bounds retries of transient failures. Default 3; negative disables.
	MaxRetries int
	// MaxRepairs bounds re-asking the model after a schema rejection. Default 2; negative disables.
	MaxRepairs int
	// Backoff is the first retry delay; it doubles per retry. Default 500ms.
	Backoff time.Duration
	// HTTPClient overrides the HTTP transport.
	HTTPClient *http.Client

	// MaxChunkTokens is the token budget per LLM request. Default 6000.
	MaxChunkTokens int
	// Encoding is the tiktoken encoding used to count tokens. Default "cl100k_base".
	Encoding string
	// Exclude holds path globs to skip. Nil selects the defaults
	// (*.pb.go, *_gen.go, go.sum, vendor/**, *.lock); an empty slice skips nothing.
	Exclude []string
	// Concurrency bounds parallel LLM requests. Default 4.
	Concurrency int
}
