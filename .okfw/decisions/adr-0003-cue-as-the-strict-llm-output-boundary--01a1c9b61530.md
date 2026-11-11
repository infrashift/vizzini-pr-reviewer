---
id: "01a1c9b6-1530-7f30-84bb-849fd2eb8a2b"
title: "ADR-0003: CUE as the strict LLM output boundary"
type: "ADR"
tags:
  - "adr"
  - "cue"
  - "schema"
  - "llm"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Every LLM response is unified with schema/review.cue via the CUE Go API before any data reaches callers."
---

# ADR-0003: CUE as the strict LLM output boundary

**Status:** Accepted
**Date:** 2026-11-11
**Source:** `CLAUDE.md`, `PLAN.md`

## Context
LLM output is untrusted and nondeterministic. Callers need typed data with a guaranteed shape: the `mrman` TUI renders findings, and CI decides exit codes from severities (Milestone 4.1). Constraints like `end_line >= start_line` or a closed severity enum have to hold every time.

## Decision
- `schema/review.cue` is the canonical schema and is embedded with `//go:embed`.
- Every LLM response is unified against it with `cuelang.org/go/cue/cuecontext` and decoded into Go structs **before** it's returned. Invalid output is an error, never passed through.
- The same CUE package will also hold the policy gate (4.1), path risk weights (4.4) and `#ImplementationPlan` (4.7).
- The shape is specified in [Review output schema](/specs/review-output-schema--01a1c9b61543.md).

## Consequences
- There is one source of truth for output shape and policy, checked by `make vet-cue`.
- A model that drifts from the enum (e.g. `"High"` instead of `"Major"`) gets rejected. The LLM client's retry/repair path has to handle that.
- Concurrency caveat (found in okf-wiki): CUE's runtime is written to during unification, so a shared `cue.Context` must be serialized or built per call. `go test -race` stays on.

## Alternatives considered
- **JSON Schema.** Rejected: it's an extra dependency and expresses cross-field constraints poorly.
- **Go struct tags plus hand validation.** Rejected: the rules end up scattered and can't be shared with policy configuration.
