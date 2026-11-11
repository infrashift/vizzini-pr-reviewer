---
id: "01a1c9b6-156a-7d62-8393-ef4d75e2dccb"
title: "ISSUE-005: Hybrid verification: static analysis plus AI"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "analysis"
  - "milestone-4-2"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:13:19-06:00"
description: "Ingest golangci-lint JSON, keep diagnostics on modified lines, and inject them into the review prompt."
---

# ISSUE-005: Hybrid verification: static analysis plus AI

**Status:** closed · **Phase:** 4 · **Milestone:** 4.2 · **PLAN.md:** Milestone 4.2: Hybrid Verification (Static Analysis + AI)

## Description
Feed linter output into the LLM context so the AI explains lint errors and proposes fixes without inventing rules.

## Acceptance Criteria
- [x] Parse `golangci-lint run --out-format=json` (or an injected `io.Reader`)
- [x] Filter diagnostics to lines modified within the active `gitdiff.File` hunks
- [x] Inject matched diagnostics into `review.tmpl` before inference
- [x] Fixture test confirms only overlapping diff lines are injected

## Notes
- golangci-lint v2 changed its output flags (`--output.json.path`). Accept both formats through the `io.Reader` path.

## Resolution
Done 2026-11-11.
- **Confirmed against the installed golangci-lint v2.15:** `--out-format` is gone in v2, and JSON comes from `--output.json.path=stdout`. The `Issues[]` shape (`FromLinter`, `Text`, `Pos.Filename/Line/Column`) is unchanged, so one parser reads both. The fixture is modeled on real v2 output.
- **`internal/analysis/linter.go`:** `ParseGolangciJSON(io.Reader)` normalizes paths (backslashes, `./`, `Clean`) and skips issues with no file or line. `OnChangedLines(diags, files)` keeps only diagnostics on `+` lines. "Modified" means added on the new side: context lines, lines outside the hunks and untouched files are dropped. The results are sorted by file, line and column.
- **Path contract:** diagnostic paths must be repository-relative, so run golangci-lint from the repo root. No suffix matching is done, because it could pin a diagnostic on the wrong file of the same name.
- **Engine:** `ReviewOptions.LintReport io.Reader`. It is parsed once before any request, so a malformed report fails fast with zero LLM calls. Each chunk gets only the diagnostics on its own hunks: a split file builds a per-part `diff.File` from the part's hunks. `ReviewOutput.Diagnostics` counts what was injected.
- **Prompt:** an optional `<lint>` block in `review.user`, before the diff, tells the model this is real tool output to explain and fix, and not to invent more.
- **CLI:** `--lint-json FILE` is review mode only. `-` is rejected because stdin is reserved for the patch.
- **Tests:** the fixture (5 issues: an added line, a context line, a line outside the hunk, a `./`-prefixed path on a new file, an untouched file) yields exactly the 2 changed-line diagnostics. Engine tests check the exact `<lint>` block, per-chunk isolation across 2 chunks, no block without a report, and a malformed report making no requests. The CLI tests check prompt injection, the diagnostics count in the JSON and flag errors. A template test pins the block's format and its position before the diff.

## End-to-end proof (2026-11-11)
`make e2e-lint` (`scripts/e2e-golint.sh`) builds a throwaway Go repo whose base commit has pre-existing lint issues, applies a change that adds new ones, and runs the **real** golangci-lint v2.15 from the repo root, the real `git diff` and the real `bin/pr-review`. With no proxy configured, `scripts/e2e/stubllm` answers with one finding per `<lint>` entry. That proves the plumbing, not a model's prose; set `VIZZINI_LITELLM_URL`/`VIZZINI_MODEL` to run the same script against a real model. Result: 4 real diagnostics; the 2 on added lines reached the prompt and the 2 pre-existing ones (lines 10–11) did not. Both findings were schema-validated and anchored. gosec G304 mapped to Major and the default gate exited 2; `auto_approve` was false. Python asserts the added lines independently from the raw patch.

Learned from the real run:
- golangci-lint v2 appends its stats text (`2 issues: * errcheck: 2`) to stdout after the JSON unless `--show-stats=false` is passed. `ParseGolangciJSON` reads only the first JSON value, so it already tolerated this; `TestParseGolangciJSON_TrailingStats` pins it, and the docs recommend the flag.
- gosec did not flag `q := "..." + name; db.QueryRow(q)` as G202 (it fires when the concatenation is in the call). The fixture relies on G304 and errcheck instead.
- Anchoring proved itself: a regex bug in the stub produced file `store/users.go:22`. Vizzini rejected it, sent it back for repair twice, then failed the review instead of emitting a misplaced comment.

## Related
- [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
