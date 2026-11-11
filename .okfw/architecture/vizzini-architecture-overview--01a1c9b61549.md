---
id: "01a1c9b6-1549-73d3-8307-7bbfbf932072"
title: "Vizzini architecture overview"
type: "Architecture"
tags:
  - "architecture"
  - "overview"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Package map, review data flow and embedding targets for the Vizzini AI code review engine."
status: "draft"
---

# Vizzini architecture overview

Vizzini is a stateless, pure-Go AI code review engine: a library (`pkg/reviewer`) embedded by the `mrman` TUI, plus a static CLI (`cmd/pr-review`) for UBI9 containers and QEMU microVMs.

## Data flow
```
unified diff (io.Reader)
  → internal/diff      parse with go-gitdiff, drop generated files, annotate old/new line numbers
  → internal/chunker   pack hunks into ≤ MaxChunkTokens (default 6000) using tiktoken-go
  → internal/analysis  (4.2–4.4) linter diagnostics on changed lines, dependency order, risk score
  → internal/okf       (4.5) load matching OKF knowledge sections
  → internal/llm       render text/template prompt → go-openai → LiteLLM proxy
  → internal/schema    unify the JSON response with schema/review.cue, decode to Go structs
  → pkg/reviewer       ReviewOutput / PRDescription / []CodeSuggestion to the caller
```

## Packages
| Package | Responsibility | Delivered by |
|---|---|---|
| `internal/diff` | Patch parsing, generated-file filter, line annotation | [ISSUE-001: Diff parsing and token-aware chunking](/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md) |
| `internal/chunker` | Token-bounded packing, hunk-boundary splitting | [ISSUE-001: Diff parsing and token-aware chunking](/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md) |
| `internal/schema` | Embedded CUE, validation, policy (4.1) | [ISSUE-002: CUE validation and LiteLLM pipeline](/issues/issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md) |
| `internal/llm` | Prompt templates, LiteLLM client, retries | [ISSUE-002: CUE validation and LiteLLM pipeline](/issues/issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md) |
| `pkg/reviewer` | Public `Engine` API, Markdown helpers, commands (4.6), planner (4.7) | [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md) |
| `cmd/pr-review` | CLI flags, output formats, exit codes | [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md) |
| `internal/analysis` | Linter ingestion, AST DAG, risk scoring | 4.2–4.4 |
| `internal/okf` | OKF index/frontmatter grounding | 4.5 |

## Decisions
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)

Output contract: [Review output schema](/specs/review-output-schema--01a1c9b61543.md).
