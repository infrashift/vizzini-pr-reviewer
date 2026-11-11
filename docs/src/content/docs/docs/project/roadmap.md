---
title: Roadmap
description: Vizzini's phased plan, from the engine core to issue-driven planning.
---

Vizzini is built one phase at a time. Each phase is tracked as an issue in the
project's OKF wiki (`.okfw/issues/`) and lands as its own commit, with tests
and lint passing.

## MVP core

| Phase | Scope | Status |
|---|---|---|
| 1 | Diff parsing, generated-file filtering, line annotation, token-aware chunking | Done |
| 2 | `schema/review.cue`, CUE validator, LiteLLM client with retries and schema repair | Done |
| 3 | `pkg/reviewer` API (`Describe`, `Review`, `Improve`), Markdown helpers for mrman, `pr-review` CLI | Done |

## Advanced capabilities

| Milestone | Capability |
|---|---|
| 4.1 Policy gate (done) | `#PolicyGate` in CUE. `pr-review` exits `2` when findings hit `fail_on` (default Critical and Major) |
| 4.2 Hybrid verification (done) | `golangci-lint` JSON diagnostics on changed lines are added to the prompt, so the model explains real lint errors |
| 4.3 Dependency walkthroughs (done) | Changed Go files ordered by an AST dependency graph (models → services → UI) for mrman's file tree |
| 4.4 Risk scoring (done) | A deterministic 0–100 blast-radius score from lines changed, complexity delta and CUE path weights. `AutoApprove` only for low risk with no Critical or Major findings |
| 4.5 OKF grounding (done) | Matches changed paths against an Open Knowledge Format wiki and cites design decisions in findings |
| 4.6 Slash commands (done) | `/explain`, `/refactor` and `/docstrings` scoped to a file and line range inside a mrman review thread |
| 4.7 Pre-code planning (done) | Turns an issue payload into a CUE-validated implementation plan for coding agents |
| 4.8 Unattended PR review (done) | `vizzini-bot` reviews every push from CI, incrementally. It posts through mrman under a committed garmr policy, remembers its threads (no duplicates, resolves fixed findings), answers `/vizzini` commands in PR threads, and honors suppression rules from the base branch. See [Unattended PR Review](../../guides/bot/) |
