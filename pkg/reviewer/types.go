package reviewer

import (
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/internal/analysis"
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
	CategoryArchitecture    = schema.CategoryArchitecture
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

// FileRisk is one file's contribution to the deterministic risk score.
type FileRisk = analysis.FileRisk

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
	// RiskScore is max(BaseRiskScore, ModelRiskScore), in 0..100: the model
	// can raise the risk of a change but never lower it below what the diff
	// itself shows.
	RiskScore int `json:"risk_score"`
	// BaseRiskScore is the deterministic blast-radius score computed from the
	// diff and the risk policy (Milestone 4.4).
	BaseRiskScore int `json:"base_risk_score"`
	// ModelRiskScore is the highest risk score any chunk's answer reported.
	ModelRiskScore int `json:"model_risk_score"`
	// RiskFiles explains BaseRiskScore per file, highest contribution first.
	RiskFiles []FileRisk `json:"risk_files,omitempty"`
	// AutoApprove is true only when something was reviewed, nothing was
	// skipped, no touched file is sensitive (weight at or above the policy's
	// sensitive_weight), RiskScore is at or below auto_approve_threshold, and
	// there are no Critical or Major findings.
	AutoApprove bool `json:"auto_approve"`
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
	// added to the prompts (see ReviewOptions.LintReports).
	Diagnostics int `json:"diagnostics,omitempty"`
	// Attribution identifies the run (RunID is always set) and whom it was
	// for, so the report joins the proxy's spend logs (ISSUE-025).
	Attribution
	// Usage is what this review's model requests consumed (ISSUE-025).
	Usage *Usage `json:"usage,omitempty"`
	// ContextSnippets counts the repository-context snippets (definitions
	// and callers outside the diff) given to the model (ISSUE-023).
	ContextSnippets int `json:"context_snippets,omitempty"`
	// MergedFindings counts same-span duplicates folded into one finding:
	// a problem the model reported both by its nature and as the knowledge
	// document it violates (ISSUE-019, ADR-0012).
	MergedFindings int `json:"merged_findings,omitempty"`
	// KnowledgeDocs lists the IDs of the knowledge-base documents given to
	// the model (see ReviewOptions.Knowledge), sorted.
	KnowledgeDocs []string `json:"knowledge_docs,omitempty"`
	// Suppressed are findings a ReviewOptions.Suppressions rule matched:
	// not in Findings, so not posted and not gated on, but counted against
	// AutoApprove (ISSUE-024 slice 5).
	Suppressed []SuppressedFinding `json:"suppressed,omitempty"`
	// Filtered are findings below the engine's noise floor (see
	// Config.NoiseFloor): not in Findings, so not posted and not gated on,
	// but counted against AutoApprove like Suppressed. Empty when
	// ReviewOptions.AllFindings is set.
	Filtered []Finding `json:"filtered,omitempty"`
}

// ReviewOptions tunes a single Review call.
type ReviewOptions struct {
	// Suppressions, if set, are the repository's rules for findings it
	// decided not to be told about again (see ParseSuppressions).
	Suppressions *Suppressions
	// AllFindings turns the noise floor off for this review: every finding
	// stays in ReviewOutput.Findings.
	AllFindings bool
	// SkipOversized leaves out any hunk larger than the token budget instead
	// of sending it in a chunk of its own. Skipped files are reported in
	// ReviewOutput.Skipped.
	SkipOversized bool
	// Knowledge, if set, is an OKF bundle (e.g. os.DirFS(".okfw")). The
	// documents whose paths, domain or tags match a chunk's files are given to
	// the model, within Config.MaxKnowledgeTokens per chunk, so it can cite
	// design decisions; Architecture findings must cite one of them.
	Knowledge fs.FS
	// LintReports are linter reports produced from the repository root, each
	// either golangci-lint JSON (v1 --out-format=json, v2
	// --output.json.path=stdout) or SARIF 2.1.0 (ruff, Roslyn analyzers,
	// Semgrep, CodeQL, …); the format is detected per report. Diagnostics on
	// lines the diff adds are given to the model so it can explain them and
	// propose fixes; all others are ignored.
	LintReports []io.Reader
	// LintRoot is the repository root as an absolute path. SARIF file://
	// URIs (and absolute golangci-lint paths) under it are made relative;
	// absolute locations outside it are ignored.
	LintRoot string
	// Repo, if set, is the repository at the change's head (e.g.
	// os.DirFS(root)). The definitions of symbols a chunk uses and the call
	// sites of symbols it changes are read from it and given to the model
	// as read-only context, within Config.MaxContextTokens per chunk
	// (ISSUE-023). Findings must still be on lines of the diff.
	Repo fs.FS
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
	// MaxKnowledgeTokens bounds the knowledge-base text added to each chunk's
	// prompt. Default 2000.
	MaxKnowledgeTokens int
	// MaxContextTokens bounds the repository context (ReviewOptions.Repo)
	// added to each chunk's prompt. Default 1500.
	MaxContextTokens int
	// RiskPolicy is a JSON document validated against #RiskPolicy in
	// schema/review.cue (path weights, scale, auto_approve_threshold).
	// Omitted fields, or a nil RiskPolicy, take the schema defaults.
	RiskPolicy []byte
	// NoiseFloor is a JSON document validated against #NoiseFloor: the
	// kinds of finding (severity, optionally narrowed to categories) moved
	// from ReviewOutput.Findings to ReviewOutput.Filtered. Nil selects the
	// schema default, which filters Trivial findings and Minor
	// Maintainability and Performance ones (ISSUE-032, ADR-0024);
	// {"rules": []} filters nothing.
	NoiseFloor []byte
}
