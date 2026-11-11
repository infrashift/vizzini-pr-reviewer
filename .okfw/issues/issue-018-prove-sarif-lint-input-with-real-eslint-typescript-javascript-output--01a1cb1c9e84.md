---
id: "01a1cb1c-9e84-7e31-9aab-fce99d9b3e10"
title: "ISSUE-018: Prove SARIF lint input with real ESLint (TypeScript/JavaScript) output"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "analysis"
  - "lint"
  - "sarif"
  - "typescript"
  - "javascript"
  - "closed"
created: "2026-11-11T08:56:52-06:00"
timestamp: "2026-11-11T08:56:52-06:00"
description: "Add TypeScript/JavaScript lint support by proving ESLint + typescript-eslint SARIF (via @microsoft/eslint-formatter-sarif) end to end, with a captured fixture, docs, and a seeded TypeScript fixture in the vizzini-uat live suite."
---

# ISSUE-018: Prove SARIF lint input with real ESLint (TypeScript/JavaScript) output

**Status:** closed · **Raised:** 2026-11-11 · **Follows:** [ISSUE-014](/issues/issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md) (ruff, Roslyn) · **Parent:** [ISSUE-012](/issues/issue-012-multi-language-lint-input-via-sarif--01a1c9ed12c7.md)

## Why
TypeScript and JavaScript are the last major languages Vizzini reviews without real linter grounding. We want one linter, the most widely used one that can write SARIF 2.1.0.

## Decision
- **ESLint**, roughly 130M weekly npm downloads in mid-2026 against about 9M for Biome and 7M for oxlint. **typescript-eslint** handles TypeScript, including type-aware rules.
- ESLint has no native SARIF output, so we use **`@microsoft/eslint-formatter-sarif`**: `eslint -f @microsoft/eslint-formatter-sarif -o eslint.sarif .`
- **Fallback:** Biome writes SARIF natively (`biome check --reporter=sarif`) if the formatter stops working with current ESLint. Out of scope unless that happens.
- **Pins** (each release at least two weeks old):

| package | version |
|---|---|
| `node` image | 24.22.1-alpine, pinned by digest |
| eslint | 10.13.0 |
| @eslint/js | 10.0.1 |
| typescript-eslint | 8.72.1 |
| typescript | 6.0.3 |
| @microsoft/eslint-formatter-sarif | 3.1.0 |

- **typescript pin:** typescript-eslint 8.72 requires `typescript <6.1.0`, so TypeScript 7 is excluded.

## Risks to verify against real output
- The formatter (3.1.0, April 2024) lists `eslint ^8.9.0` as a regular dependency. It installs an end-of-life ESLint 8 next to ESLint 10. Does it still format ESLint 10 results correctly?
- URI shape: absolute `file://` like ruff and Roslyn, or relative?
- `eslint-disable` suppressions, parse errors with no `ruleId`, "file ignored" warnings with no line, and `ruleIndex`/description lookup.

## Acceptance criteria
- [x] `scripts/e2e-jslint.sh` and `make e2e-lint-js`: a throwaway TypeScript repo whose base already has a lint warning, and a change that adds security (`no-eval`, `no-implied-eval`, `no-new-func`), `@typescript-eslint/no-unused-vars` and type-aware (`no-floating-promises`) diagnostics. Real ESLint SARIF from the pinned node image under podman, with the repo mounted at its own absolute path. Shared `assert_lint.py`, using stubllm or a live model
- [x] Fixture `internal/analysis/testdata/sarif/eslint-10.13.0.sarif` with the root rewritten to `/work/repo`, plus `TestParseSARIF_RealESLint` (including that the wrong root drops everything). Parser changes only if real output needs them, test first
- [x] stubllm counts ESLint security rule IDs as Major
- [x] Lint guide: a JS/TS row with the verified version, the command and any quirks. `make-targets.md`: `e2e-lint-js`
- [x] vizzini-uat: a seeded TypeScript fixture with ground truth and a clean decoy, run through the live Sonnet/Opus + garmr + mrman + ADO suite ([ISSUE-016](/issues/issue-016-cross-tool-uat-on-c-with-live-claude-sonnet-5-5-and-opus-5-5--01a1caee2585.md))

## Out of scope (known gaps, same as C#)
- The risk score's complexity term is Go-only, so TypeScript files score `complexity_delta: 0`.
- Knowledge grounding matches only Go `package` clauses. TypeScript knowledge bases need `x.paths` globs.

## Progress (2026-11-11)
The real toolchain works without parser changes: ESLint 10.13.0 + typescript-eslint 8.72.1 + `@microsoft/eslint-formatter-sarif` 3.1.0 on Node 24.22.1.
- **Format:** SARIF 2.1.0 with absolute `file://` URIs, `artifacts` and `ruleIndex`.
- **Suppressions:** `eslint-disable` produces `suppressions: [{"kind": "inSource"}]` with no status. Vizzini already treats that as suppressed, and the e2e now proves a suppressed result on an *added* line stays out of the prompt.
- **ESLint 10:** the formatter's dependency on `eslint ^8.9.0` installs an old ESLint 8 copy, but that copy isn't used for output: the run's driver version reads 10.13.0.
- **Shared assertions:** `assert_lint.py` now reads SARIF suppressions on its own terms (SARIF 3.35: no status or `accepted` means in effect). The Go, Python and C# proofs still pass.
- **Stub, stub mode:** the 4 code-execution diagnostics become Major and the gate exits 2.

## Resolution (2026-11-11)
TypeScript/JavaScript lint input is supported through ESLint SARIF. The proof, fixture test and docs are listed under Progress. The live suite ran the seeded TypeScript fixture in vizzini-uat (run 20261111T150647Z, `results/2026-11-11-live-typescript-sonnet-opus-5-5.md`) and **passed 46/46 hard checks**.

| | Sonnet 5.5 | Opus 5.5 |
|---|---|---|
| Seeded defects caught | 10/10 | 10/10 |
| Category agreement | 100% | 100% |
| Findings (Critical/Major/Minor/Trivial) | 11 (4/6/1/0) | 15 (4/9/2/0) |
| Findings on the clean decoy | 0 | 0 |
| Risk (final/diff/model) | 95/46/95 | 97/46/97 |
| garmr | deny VZ-001, VZ-002, VZA-001 | same |
| Review / describe / improve / plan | 18s / 7s / 21s / 18s | 24s / 8s / 31s / 17s |
| ADO inline threads (PRs #6 and #7) | 11 | 15 |

- **ESLint signal:** only 2 of the 10 defects were visible to ESLint (`no-eval`, `no-floating-promises`). The other 8 came from the models: command injection, prototype pollution, timing-unsafe MAC comparison, ReDoS, return-without-await, off-by-one, the check-then-act race, and the hardcoded token.
- **Unplanted catch:** both models also flagged the empty-string HMAC secret fallback. It isn't one of the planted defects, but the fixture's secrets ADR forbids it, so it is a correct finding.

### Follow-ups
- **Duplicate findings** (now [ISSUE-019](/issues/issue-019-deduplicate-same-span-findings-security-architecture-pairs--01a1cb341ed5.md)). Opus sometimes files one problem twice on the same lines, once as Security and once as Architecture citing the ADR (eval, token, execSync, secret handling: 4 pairs). Vizzini's deterministic merge (ADR-0007) only merges across chunks, not within one answer. A same-span, same-root-cause dedupe, or a prompt rule, would cut the noise on PRs.
- The Go-only complexity and grounding gaps from ISSUE-016 apply to TypeScript too.
