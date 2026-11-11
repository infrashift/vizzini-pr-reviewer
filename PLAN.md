# PLAN.md — Go AI Code Review Engine Implementation Plan

Phases are mirrored as issues in the project OKF wiki (`.okfw/issues/`). See [Wiki conventions](.okfw/wiki-conventions--01a1c9b61519.md). When a phase is done, tick both this checklist and its issue.

## Target Repository Layout
```text
.
├── CLAUDE.md
├── PLAN.md
├── .okfw/                     # OKF wiki: architecture, specs, ADRs, issues, daily notes
├── Makefile
├── go.mod
├── cmd/
│   └── pr-review/
│       └── main.go              # CLI entrypoint for container/microVM execution
├── schema/
│   └── review.cue               # Canonical CUE schemas for config, findings, and severity
├── internal/
│   ├── diff/
│   │   ├── parser.go            # go-gitdiff wrapper & generated-file filter
│   │   └── parser_test.go
│   ├── chunker/
│   │   ├── chunker.go           # Token-aware diff splitting via tiktoken-go
│   │   └── chunker_test.go
│   ├── schema/
│   │   ├── validator.go         # CUE unification & Go struct decoding
│   │   └── validator_test.go
│   ├── llm/
│   │   ├── client.go            # LiteLLM HTTP client & text/template prompt builder
│   │   ├── templates/           # Embedded .tmpl prompts (describe, review, improve)
│   │   └── client_test.go
│   ├── analysis/
│   │   ├── linter.go            # Hybrid static analysis runner (JSON stdout ingestion)
│   │   ├── dag.go               # Go AST dependency graph & topological sort
│   │   └── risk.go              # Blast radius & file-weight calculator
│   └── okf/
│       └── grounder.go          # Open Knowledge Format (OKF) frontmatter & index.md loader
└── pkg/
    └── reviewer/
        ├── engine.go            # Public Go API consumed by the mrman TUI
        └── types.go             # Exported domain structs

```

---

## Phase 1: MVP Core — Diff Parsing & Token-Aware Chunking

**Tracking:** [ISSUE-001](.okfw/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md)

**Goal:** Parse raw unified diffs, strip noisy generated artifacts, and pack file hunks into token-bounded batches without splitting individual hunks mid-line.

### Implementation Tasks

* [x] Initialize `go.mod` and create the `Makefile` with `test`, `vet-cue`, `lint`, and `build` targets.
* [x] Implement `internal/diff/parser.go` using `github.com/bluekeyes/go-gitdiff`:
* Parse unified `.patch` streams into structured file and text fragments.
* Exclude generated paths matching configurable globs (`*.pb.go`, `*_gen.go`, `go.sum`, `vendor/**`, `*.lock`).
* Annotate each hunk line with explicit old/new line numbers so the LLM can anchor comments accurately.


* [x] Implement `internal/chunker/chunker.go` using `github.com/pkoukk/tiktoken-go` (`cl100k_base` / `o200k_base`):
* Pack parsed files into `Chunk` slices where total tokens remain below `MaxChunkTokens` (default: 6000).
* If a single file exceeds `MaxChunkTokens`, split at hunk boundaries (`*gitdiff.TextFragment`).



### Definition of Done (Verification)

* [x] `make test` passes unit tests in `internal/diff` and `internal/chunker` covering binary diffs, ignored lockfiles, multi-file packing, and oversized single-file hunk splitting.

---

## Phase 2: MVP Core — CUE Validation & LiteLLM Pipeline

**Tracking:** [ISSUE-002](.okfw/issues/issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md)

**Goal:** Enforce deterministic schemas on all LLM outputs and wire prompt execution through the LiteLLM proxy.

### Implementation Tasks

* [x] Author `schema/review.cue` defining `#Severity`, `#Category`, `#Finding`, and `#ReviewOutput`:
```cue
package review

#Severity: "Critical" | "Major" | "Minor" | "Trivial"
#Category: "Security" | "Performance" | "Logic" | "Maintainability"

#Finding: {
    file:       string & !=""
    start_line: int & >=1
    end_line:   int & >=start_line
    severity:   #Severity
    category:   #Category
    title:      string & !=""
    body:       string & !=""
    suggestion?: string
}

#ReviewOutput: {
    summary:    string
    risk_score: int & >=0 & <=100
    findings:   [...#Finding]
}

```


* [x] Implement `internal/schema/validator.go` embedding `schema/review.cue` via `//go:embed` and validating raw JSON bytes using `cuecontext.New()`.
* [x] Implement `internal/llm/client.go` using `text/template` and `go-openai` configured with custom `BaseURL` (for LiteLLM / mock Anthropic endpoints), retry backoff, and JSON output mode.

### Definition of Done (Verification)

* [x] `make vet-cue` succeeds with zero warnings.
* [x] `make test` passes table-driven tests verifying that malformed LLM JSON or invalid severity strings (e.g., `"High"` instead of `"Major"`) are rejected by the CUE validator.

---

## Phase 3: MVP Core — Public API (`pkg/reviewer`) & CLI Entrypoint

**Tracking:** [ISSUE-003](.okfw/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)

**Goal:** Expose the `/describe`, `/review`, and `/improve` workflows as both an importable Go package for the `mrman` TUI and a standalone static CLI binary.

### Implementation Tasks

* [x] Implement `pkg/reviewer/engine.go` with methods:
* `Describe(ctx context.Context, patch io.Reader) (*PRDescription, error)`
* `Review(ctx context.Context, patch io.Reader, opts ReviewOptions) (*ReviewOutput, error)`
* `Improve(ctx context.Context, patch io.Reader) ([]CodeSuggestion, error)`


* [x] Add Markdown formatting helpers in `pkg/reviewer` tailored for `bubbletea` and `lipgloss` viewport rendering.
* [x] Implement `cmd/pr-review/main.go` accepting `--patch-file`, `--mode` (`describe|review|improve`), `--litellm-url`, and `--format` (`json|markdown`).

### Definition of Done (Verification)

* [x] `make build` produces a static binary (`CGO_ENABLED=0`) verified via `file bin/pr-review`.
* [x] End-to-end integration test runs `Engine.Review` against an `httptest.Server` mocking the LiteLLM proxy.

---

## Phase 4: Advanced Roadmap Capabilities

Execute each sub-milestone below as a separate commit cycle after Phases 1–3 are complete.

### Milestone 4.1: Triage & Severity Policy Gate

**Tracking:** [ISSUE-004](.okfw/issues/issue-004-triage-and-severity-policy-gate--01a1c9b61563.md)

* **User Story Context:** Tag findings with a CUE-enforced severity enum (`Critical`, `Major`, `Minor`, `Trivial`) so blocking policies only trigger on high-risk issues.
* **Technical Spec (`internal/schema/policy.go`):**
* [x] Add `#PolicyGate` in `schema/review.cue` specifying `fail_on: [...#Severity]` (default: `["Critical", "Major"]`).
* [x] Return exit code `2` in `cmd/pr-review` when findings match `fail_on`, and `0` when only `Minor` or `Trivial` items exist.


* **Verification:** Unit tests verifying exit codes and policy evaluation across all severity permutations.

### Milestone 4.2: Hybrid Verification (Static Analysis + AI)

**Tracking:** [ISSUE-005](.okfw/issues/issue-005-hybrid-verification-static-analysis-plus-ai--01a1c9b6156a.md)

* **User Story Context:** Ingest stdout from standard Go linters and AST scanners into the LLM context so the AI explains syntax/lint errors and proposes fixes without hallucinating rules.
* **Technical Spec (`internal/analysis/linter.go`):**
* [x] Parse JSON diagnostics from `golangci-lint run --out-format=json` (or injected `io.Reader`).
* [x] Filter diagnostics to only lines modified within the active `gitdiff.File` hunks.
* [x] Inject matched linter diagnostics into the `review.tmpl` prompt section prior to LLM inference.


* **Verification:** Fixture test feeding sample `golangci-lint` JSON output and confirming prompt injection only includes overlapping diff lines.

### Milestone 4.3: Semantic Dependency Walkthroughs

**Tracking:** [ISSUE-006](.okfw/issues/issue-006-semantic-dependency-walkthroughs--01a1c9b61571.md)

* **User Story Context:** Order Pull Request diffs by architectural dependencies (e.g., data models first, then backend logic, then UI) for review in the `mrman` TUI.
* **Technical Spec (`internal/analysis/dag.go`):**
* [ ] Parse modified `.go` files with `go/parser` and `go/ast` to extract package imports and struct/interface declarations.
* [ ] Construct a Directed Acyclic Graph (DAG) of modified files and run Kahn's topological sort algorithm (breaking cycles deterministically by path depth).
* [ ] Expose `OrderFilesByDependency(files []*gitdiff.File) []*gitdiff.File` for the `mrman` TUI file tree.


* **Verification:** Unit test passing a 3-layer diff (`ui.go` -> `service.go` -> `model.go`) and asserting the returned slice orders `model.go` first.

### Milestone 4.4: Automated Risk Scoring & Approvals

**Tracking:** [ISSUE-007](.okfw/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)

* **User Story Context:** Calculate a blast radius score based on touched files so low-risk changes can be automatically approved.
* **Technical Spec (`internal/analysis/risk.go`):**
* [ ] Compute a deterministic base score (`0–100`) combining hunk line count, cyclomatic complexity delta, and path risk weights defined in CUE (e.g., `auth/**` or `schema/**` = high weight; `docs/**` or `*_test.go` = low weight).
* [ ] Emit `AutoApprove: true` in `ReviewOutput` only when `RiskScore <= threshold` AND zero `Critical`/`Major` findings exist.


* **Verification:** Table-driven tests validating risk calculation and auto-approval gating on documentation-only vs. security-critical diffs.

### Milestone 4.5: Knowledge Base Grounding (OKF RAG)

**Tracking:** [ISSUE-008](.okfw/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md)

* **User Story Context:** Cross-reference the internal Open Knowledge Format (OKF) Markdown directory to flag architectural violations and cite specific design decisions.
* **Technical Spec (`internal/okf/grounder.go`):**
* [ ] Parse progressive-disclosure root `index.md` files and YAML frontmatter (`tags`, `domain`, `paths`) across the target OKF directory.
* [ ] Match modified diff file paths and package names against OKF frontmatter rules to load only relevant Markdown sections into the prompt context.
* [ ] Require `#Finding` citations to include the matched OKF document ID when an architectural constraint is violated.


* **Verification:** Test fixture with a mock OKF directory (`index.md` + 2 domain docs) verifying selective context loading based on diff paths.

### Milestone 4.6: Interactive PR Commands

**Tracking:** [ISSUE-009](.okfw/issues/issue-009-interactive-pr-slash-commands--01a1c9b61586.md)

* **User Story Context:** Invoke slash commands (`/explain`, `/refactor`, `/docstrings`) directly within the `mrman` review thread.
* **Technical Spec (`pkg/reviewer/commands.go`):**
* [ ] Implement a command parser supporting `/explain [file:line-range]`, `/refactor [file:line-range]`, and `/docstrings [file]`.
* [ ] Route each command to a dedicated template in `internal/llm/templates/` scoped strictly to the selected hunk and thread history.


* **Verification:** Unit tests verifying command parsing, line-range extraction, and template rendering.

### Milestone 4.7: Pre-Code Planning (Issue Tracker Sync)

**Tracking:** [ISSUE-010](.okfw/issues/issue-010-pre-code-planning-from-issue-payloads--01a1c9b6158d.md)

* **User Story Context:** Analyze an issue ticket and generate a structured implementation plan before code is written.
* **Technical Spec (`pkg/reviewer/planner.go`):**
* [ ] Accept an `IssuePayload` struct (title, body, labels) alongside a repository file tree summary and OKF `index.md`.
* [ ] Output a CUE-validated `#ImplementationPlan` containing affected files, phased steps, and verification commands formatted for direct handoff to coding agents.


* **Verification:** Unit test validating `#ImplementationPlan` CUE unification against a mocked issue payload.

## Tools that we may want to integration with

* Open Knowledge Format Wiki - https://github.com/infrashift/okf-wiki/

NOTE: okfw binary is installed at ~/.local/bin/okfw. If you want to evaluate it's use and create an OKF Wiki instance in this repo to track architecture, SPECs, ADRs, ISSUEs, daily notes, etc. Also, you can create a test okf wiki instance to use for testing this PR Review tool.

* PR management TUI - https://github.com/infrashift/mrman/
  

