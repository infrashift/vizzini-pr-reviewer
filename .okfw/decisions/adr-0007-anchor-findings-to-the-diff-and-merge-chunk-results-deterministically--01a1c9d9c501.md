---
id: "01a1c9d9-c501-7a64-be3a-c62758f875f6"
title: "ADR-0007: Anchor findings to the diff and merge chunk results deterministically"
type: "ADR"
tags:
  - "adr"
  - "api"
  - "llm"
  - "schema"
created: "2026-11-11T03:04:14-06:00"
timestamp: "2026-11-11T03:04:14-06:00"
description: "Findings and suggestions must point at files and new-line ranges the model saw; chunk results merge into one sorted, deterministic output."
---

# ADR-0007: Anchor findings to the diff and merge chunk results deterministically

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-003](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md); extends [ADR-0003](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)

## Context
The CUE schema guarantees an answer's *shape*, not that it refers to the diff. A model can return a well-formed finding for a file it never saw, or for line 120 when the hunk covers 41–47. mrman anchors comments at those lines, so a wrong anchor is a wrong comment. Large patches are also reviewed in several chunks, which then have to be combined into one answer.

## Decision
- **Anchoring is part of validation.** After CUE validation, every finding (review) and suggestion (improve) must name a file in *that chunk*. Its `start_line..end_line` must also overlap a changed hunk's new-side range. Files with no new-side lines (deleted, binary, header-only) accept any range. Describe's `changes[].file` must name a file in the diff.
- **A failure is treated like a schema failure.** It wraps `schema.ErrInvalid`, and the message names the field, index and allowed ranges (e.g. `findings.0: lines 120-121 of internal/store/users.go are outside the changed hunks (new lines 41-47)`). The model gets it back through the existing repair loop.
- **Deterministic merge.** Findings are sorted by severity rank, then file, line and title. Suggestions are sorted by file and line. `risk_score` is the maximum across chunks. With several chunks, the summaries are joined as `Part i of n: …` paragraphs, with no extra LLM call.
- **Chunks run in parallel** up to `Config.Concurrency` (default 4). The first failure cancels the rest and is reported as `chunk i of n: …`.
- **Oversized hunks** are sent alone by default. `ReviewOptions.SkipOversized` drops them and lists them in `ReviewOutput.Skipped`.
- **Public types.** `Finding`, `PRDescription` and `CodeSuggestion` alias the schema types, so they always match the CUE shape. `ReviewOutput` is its own struct, because it carries engine-computed fields (`Excluded`, `Skipped`, `Chunks`, and later `AutoApprove` for Milestone 4.4) that the closed LLM definition must not allow.
- **CLI.** Exit `0` means success and `1` means any error, including usage errors (Go's `flag` default of 2 is not used). Exit `2` is reserved for the policy gate (4.1). The API key comes only from `VIZZINI_API_KEY` or `LITELLM_API_KEY`, never a flag, so it can't show up in `ps`.

## Consequences
- Callers never receive findings that can't be placed on a line in the diff.
- A model that keeps pointing outside the diff exhausts its repairs, and the whole review fails rather than returning partial data.
- A multi-chunk summary is a concatenation, not a synthesis. A synthesis pass can be added later behind an option.

## Alternatives considered
- **Silently dropping unanchored findings.** Rejected: it hides model errors and loses information the repair loop can usually recover.
- **Remapping out-of-range lines to the nearest hunk.** Rejected: it guesses, and a confidently misplaced comment is worse than an error.
