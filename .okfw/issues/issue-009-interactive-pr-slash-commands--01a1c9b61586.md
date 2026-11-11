---
id: "01a1c9b6-1586-76e1-9fd7-b8c0f997ed8e"
title: "ISSUE-009: Interactive PR slash commands"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "commands"
  - "milestone-4-6"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Parse /explain, /refactor and /docstrings with file:line-range targets and route each to a scoped template."
---

# ISSUE-009: Interactive PR slash commands

**Status:** open · **Phase:** 4 · **Milestone:** 4.6 · **PLAN.md:** Milestone 4.6: Interactive PR Commands

## Description
Let users run slash commands (`/explain`, `/refactor`, `/docstrings`) directly within the `mrman` review thread.

## Acceptance Criteria
- [ ] Command parser for `/explain [file:line-range]`, `/refactor [file:line-range]`, `/docstrings [file]`
- [ ] Each command routes to a dedicated `internal/llm/templates/` template scoped to the selected hunk and thread history
- [ ] Unit tests for parsing, line-range extraction and template rendering

## Related
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
