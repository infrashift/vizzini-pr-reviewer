---
id: "01a1c9b6-156a-7d62-8393-ef4d75e2dccb"
title: "ISSUE-005: Hybrid verification: static analysis plus AI"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "analysis"
  - "milestone-4-2"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Ingest golangci-lint JSON, keep diagnostics on modified lines, and inject them into the review prompt."
---

# ISSUE-005: Hybrid verification: static analysis plus AI

**Status:** open · **Phase:** 4 · **Milestone:** 4.2 · **PLAN.md:** Milestone 4.2: Hybrid Verification (Static Analysis + AI)

## Description
Feed linter output into the LLM context so the AI explains lint errors and proposes fixes without inventing rules.

## Acceptance Criteria
- [ ] Parse `golangci-lint run --out-format=json` (or an injected `io.Reader`)
- [ ] Filter diagnostics to lines modified within the active `gitdiff.File` hunks
- [ ] Inject matched diagnostics into `review.tmpl` before inference
- [ ] Fixture test confirms only overlapping diff lines are injected

## Notes
- golangci-lint v2 changed its output flags (`--output.json.path`). Accept both formats through the `io.Reader` path.

## Related
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
