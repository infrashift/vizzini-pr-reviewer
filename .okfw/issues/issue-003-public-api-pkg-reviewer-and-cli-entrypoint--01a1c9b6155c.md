---
id: "01a1c9b6-155c-7e00-b795-9e63340a2941"
title: "ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-3"
  - "api"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:04:14-06:00"
description: "Expose Describe/Review/Improve via pkg/reviewer and a static pr-review CLI with json/markdown output."
---

# ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint

**Status:** closed · **Phase:** 3 · **PLAN.md:** Phase 3: MVP Core — Public API (`pkg/reviewer`) & CLI Entrypoint

## Description
Expose the `/describe`, `/review`, and `/improve` workflows as an importable Go package for the `mrman` TUI and as a standalone static CLI binary.

## Acceptance Criteria
- [x] `Engine.Describe(ctx, patch) (*PRDescription, error)`
- [x] `Engine.Review(ctx, patch, opts) (*ReviewOutput, error)`
- [x] `Engine.Improve(ctx, patch) ([]CodeSuggestion, error)`
- [x] Markdown formatting helpers for bubbletea/lipgloss viewports
- [x] `cmd/pr-review`: `--patch-file`, `--mode`, `--litellm-url`, `--format`
- [x] `make build` yields a static binary (`file bin/pr-review`)
- [x] E2E test of `Engine.Review` against an `httptest.Server` LiteLLM mock

## Resolution
Done 2026-11-11.
- **Schema:** added `#PRType`, `#FileChange`, `#PRDescription`, `#CodeSuggestion` and `#ImproveOutput`, so describe and improve answers are validated like reviews (CLAUDE.md rule 3). There are matching Go types, `ValidateDescription`/`ValidateImprove`, and `#PRType` joins the enum drift test.
- **Templates:** `describe.tmpl` gets every file header plus the first chunk, with a truncation notice when more content exists. `improve.tmpl` works per chunk. The enum lists come from Go constants.
- **`pkg/reviewer`:** `New(Config)`, `Describe`, `Review(…, ReviewOptions)` and `Improve`. `Config` has its own fields because callers can't import `internal/llm`. Each chunk's round trip is a small method (`reviewChunk`, `improveChunk`). `ErrNoChanges` is returned by Describe when everything was excluded. Review/Improve with nothing reviewable return an empty result without calling the model.
- **Decisions:** [ADR-0007: Anchor findings to the diff and merge chunk results deterministically](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md). It covers anchoring findings to the diff, deterministic merging, bounded parallel chunks, `SkipOversized`, public type aliasing versus the engine-owned `ReviewOutput`, the CLI exit codes, and the env-only API key.
- **Markdown** (`FormatReview`, `FormatDescription`, `FormatSuggestions`): headings, lists and fenced code only. There are no tables, which overflow narrow panes, no HTML and no emoji, whose cell width breaks lipgloss layouts. A test enforces this. Code fences grow longer than any backtick run in the model's code, and fence languages come from the file extension.
- **CLI** `cmd/pr-review`: `--patch-file` (default `-`, stdin), `--mode`, `--litellm-url`, `--format` (default markdown), plus `--model`, `--max-chunk-tokens`, `--concurrency`, `--timeout` and `--skip-oversized`. Env defaults are `VIZZINI_LITELLM_URL` and `VIZZINI_MODEL`, and SIGINT/SIGTERM cancel the run. `run()` takes injected stdio and env for testing.
- **`internal/llmtest`:** a scripted mock of the LiteLLM chat endpoint that records requests and peak concurrency, plus `RunOffline` for TestMain (seeded tokenizer cache, dead outbound proxy; loopback is never proxied).
- **Verification:** `file bin/pr-review` reports `statically linked`. End-to-end tests run `Engine.Review` against the `httptest` LiteLLM mock: single chunk, multi-chunk merge, anchoring repairs (unknown file, out-of-range lines), nothing reviewable (no LLM call), `SkipOversized`, error naming and cancellation, and the concurrency bound. There are also describe (truncation plus anchoring repair) and improve tests, Markdown golden tests, and CLI tests across all modes and formats, env/flag precedence, errors (all exit 1) and help.

## Related
- [ADR-0007: Anchor findings to the diff and merge chunk results deterministically](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md)
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [Vizzini architecture overview](/architecture/vizzini-architecture-overview--01a1c9b61549.md)
- Depends on: [ISSUE-002: CUE validation and LiteLLM pipeline](/issues/issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md)
