---
id: "01a1c9b6-1556-730d-be97-8791754a9bef"
title: "ISSUE-002: CUE validation and LiteLLM pipeline"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-2"
  - "schema"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:49:18-06:00"
description: "Author schema/review.cue, the embedded CUE validator, and the go-openai LiteLLM client with templates and retries."
---

# ISSUE-002: CUE validation and LiteLLM pipeline

**Status:** closed · **Phase:** 2 · **PLAN.md:** Phase 2: MVP Core — CUE Validation & LiteLLM Pipeline

## Description
Enforce deterministic schemas on all LLM outputs and wire prompt execution through the LiteLLM proxy.

## Acceptance Criteria
- [x] Author `schema/review.cue` (`#Severity`, `#Category`, `#Finding`, `#ReviewOutput`)
- [x] `internal/schema/validator.go`: `//go:embed` the schema, validate raw JSON with `cuecontext.New()`
- [x] `internal/llm/client.go`: `text/template` prompts, go-openai with custom `BaseURL`, retry backoff, JSON mode
- [x] `make vet-cue` succeeds with zero warnings
- [x] Table-driven tests reject malformed JSON and invalid severities (e.g. `"High"`)

## Notes
- Serialize use of a shared CUE context, or build one per call (see ADR-0003). Keep `-race` green.
- `make vet-cue` currently prints a skip message while `schema/` doesn't exist (added in ISSUE-001). Once `schema/review.cue` lands it runs `cue vet ./schema/...` for real. Confirm it fails on a deliberately broken schema.
- Pinned CUE Go API: match the installed `cue` CLI (v0.17.1).

## Resolution
Done 2026-11-11.
- **Toolchain:** `cuelang.org/go v0.17.1` (matches the `cue` CLI) requires `go 1.25`, so `go.mod` moved from `go 1.23` to `go 1.25.0`. That still satisfies CLAUDE.md's "Go 1.23+". `go-openai v1.43.0`.
- **Schema:** `schema/review.cue` follows PLAN.md verbatim. `schema/embed.go` (package `schema`) exposes it as `ReviewCUE`, because `go:embed` can't reach `../../schema` from `internal/schema`. `cue vet` ignores the `.go` file. `make vet-cue` now runs for real; I checked that it fails on a deliberately broken schema.
- **`internal/schema`:** `Validator.Validate(definition, data, out)` and `ValidateReview(data)`. It rejects invalid JSON up front, then extracts with `encoding/json`, unifies with the closed definition, checks `Concrete(true)` and decodes. Every data error wraps `ErrInvalid` with one-line-per-problem CUE details, which go straight into the repair prompt. A mutex serializes CUE use (ADR-0003). The Go domain types (`ReviewOutput`, `Finding`, `Severity`, `Category`) live here so `pkg/reviewer` can alias them without an import cycle. `TestEnumsMatchSchema` reads the CUE disjunctions and fails if the Go constants drift.
- **Behavior note:** CUE treats the open list `[...#Finding]` as concrete with default `[]`, so an omitted `findings` decodes to "no findings" rather than an error. This is pinned by `TestValidateReview_MissingFindingsIsEmpty`.
- **`internal/llm`:** `Render(name, data)` runs `<name>.system` / `<name>.user` from the embedded `templates/*.tmpl` with `missingkey=error`. The enum lists come from the Go constants through template funcs. `Client.CompleteJSON(ctx, prompt, validate)` uses JSON mode and retries transient failures (network errors, 408, 429, 5xx) with doubling backoff while honoring context cancellation; 4xx is never retried. Up to `MaxRepairs` times, it sends a rejected answer back to the model with the validator error. It also strips a stray Markdown code fence. Zero-valued config fields get defaults, negative values disable the feature.
- **Templates:** only `review.tmpl` ships now. `describe` and `improve` arrive in Phase 3 together with their CUE definitions, so every LLM output always has a schema (CLAUDE.md rule 3).
- **Tests:** 21 table cases (malformed JSON, `"High"`/`"major"` severities, bad category, line constraints, risk bounds, missing/extra fields), decode round-trip, enum drift check, concurrent validation, and against an `httptest` LiteLLM mock: success with JSON mode and auth header, retry and no-retry status matrices, retry exhaustion, the repair loop, repair exhaustion wrapping `ErrInvalid`, code fences, cancellation during backoff, and template rendering.

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-001: Diff parsing and token-aware chunking](/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md)
