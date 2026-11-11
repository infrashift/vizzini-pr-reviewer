---
id: "01a1c9b6-1556-730d-be97-8791754a9bef"
title: "ISSUE-002: CUE validation and LiteLLM pipeline"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-2"
  - "schema"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Author schema/review.cue, the embedded CUE validator, and the go-openai LiteLLM client with templates and retries."
---

# ISSUE-002: CUE validation and LiteLLM pipeline

**Status:** open · **Phase:** 2 · **PLAN.md:** Phase 2: MVP Core — CUE Validation & LiteLLM Pipeline

## Description
Enforce deterministic schemas on all LLM outputs and wire prompt execution through the LiteLLM proxy.

## Acceptance Criteria
- [ ] Author `schema/review.cue` (`#Severity`, `#Category`, `#Finding`, `#ReviewOutput`)
- [ ] `internal/schema/validator.go`: `//go:embed` the schema, validate raw JSON with `cuecontext.New()`
- [ ] `internal/llm/client.go`: `text/template` prompts, go-openai with custom `BaseURL`, retry backoff, JSON mode
- [ ] `make vet-cue` succeeds with zero warnings
- [ ] Table-driven tests reject malformed JSON and invalid severities (e.g. `"High"`)

## Notes
- Serialize use of a shared CUE context, or build one per call (see ADR-0003). Keep `-race` green.
- `make vet-cue` currently prints a skip message while `schema/` doesn't exist (added in ISSUE-001). Once `schema/review.cue` lands it runs `cue vet ./schema/...` for real. Confirm it fails on a deliberately broken schema.
- Pinned CUE Go API: match the installed `cue` CLI (v0.17.1).

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-001: Diff parsing and token-aware chunking](/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md)
