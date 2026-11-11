---
id: "01a1c9b6-1586-76e1-9fd7-b8c0f997ed8e"
title: "ISSUE-009: Interactive PR slash commands"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "commands"
  - "milestone-4-6"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:35:08-06:00"
description: "Parse /explain, /refactor and /docstrings with file:line-range targets and route each to a scoped template."
---

# ISSUE-009: Interactive PR slash commands

**Status:** closed · **Phase:** 4 · **Milestone:** 4.6 · **PLAN.md:** Milestone 4.6: Interactive PR Commands

## Description
Let users run slash commands (`/explain`, `/refactor`, `/docstrings`) directly within the `mrman` review thread.

## Acceptance Criteria
- [x] Command parser for `/explain [file:line-range]`, `/refactor [file:line-range]`, `/docstrings [file]`
- [x] Each command routes to a dedicated `internal/llm/templates/` template scoped to the selected hunk and thread history
- [x] Unit tests for parsing, line-range extraction and template rendering

## Resolution
Done 2026-11-11.
- **Parser** (`pkg/reviewer/commands.go`): `ParseCommand` handles `/explain FILE:START[-END] [question]`, `/refactor FILE:START[-END] [question]` and `/docstrings FILE [note]`. Commands are case-insensitive, GitHub-style `L44-L45` ranges are accepted, and the range is split at the last colon, so paths containing `:` work. Errors wrap `ErrBadCommand` (unknown command, missing target, missing or reversed range, line 0, non-numeric line, a range on `/docstrings`).
- **Templates** (`internal/llm/templates/commands.tmpl`): dedicated `explain`, `refactor` and `docstrings` system/user templates share `thread` and `scope` sub-templates.
- **Schema (rule 3):** `/explain` uses the new `#Explanation { answer }`. `/refactor` and `/docstrings` reuse `#ImproveOutput`, so their suggestions render like Improve.
- **Strict scoping:** `RunCommand` sends only the hunks of the target file that overlap the selection (every hunk of the file for `/docstrings`), plus the newest thread messages that fit 1500 tokens. If the file isn't in the diff, or the range has no changed lines, it returns `ErrBadCommand` before any request.
- **Anchoring:** suggestions must sit in the scoped hunks. A `/refactor` suggestion must also lie *inside* the selection. The test caught that plain overlap let a 42–48 suggestion through for a 45–46 selection, so containment is checked separately with a repair message naming the allowed lines.
- **Output:** `CommandResult` and `FormatCommandResult` (Markdown for the thread). The CLI gets `--mode command --command "/…" [--thread FILE]` with a JSON thread `[{author, body}]`.
- **Tests:** parser table and error matrix, `String()`, scoping (the other hunk is absent from the prompt), thread rendering and trimming, the refactor repair sequence (other hunk, then reaching outside the selection), docstrings seeing all hunks, out-of-scope errors with zero requests, an empty explanation rejected by the schema, rendering of all three templates, Markdown, and CLI command mode in text and JSON.

## Related
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
