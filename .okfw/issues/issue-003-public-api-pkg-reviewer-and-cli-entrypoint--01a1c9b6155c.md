---
id: "01a1c9b6-155c-7e00-b795-9e63340a2941"
title: "ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-3"
  - "api"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Expose Describe/Review/Improve via pkg/reviewer and a static pr-review CLI with json/markdown output."
---

# ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint

**Status:** open · **Phase:** 3 · **PLAN.md:** Phase 3: MVP Core — Public API (`pkg/reviewer`) & CLI Entrypoint

## Description
Expose the `/describe`, `/review`, and `/improve` workflows as an importable Go package for the `mrman` TUI and as a standalone static CLI binary.

## Acceptance Criteria
- [ ] `Engine.Describe(ctx, patch) (*PRDescription, error)`
- [ ] `Engine.Review(ctx, patch, opts) (*ReviewOutput, error)`
- [ ] `Engine.Improve(ctx, patch) ([]CodeSuggestion, error)`
- [ ] Markdown formatting helpers for bubbletea/lipgloss viewports
- [ ] `cmd/pr-review`: `--patch-file`, `--mode`, `--litellm-url`, `--format`
- [ ] `make build` yields a static binary (`file bin/pr-review`)
- [ ] E2E test of `Engine.Review` against an `httptest.Server` LiteLLM mock

## Related
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [Vizzini architecture overview](/architecture/vizzini-architecture-overview--01a1c9b61549.md)
- Depends on: [ISSUE-002: CUE validation and LiteLLM pipeline](/issues/issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md)
