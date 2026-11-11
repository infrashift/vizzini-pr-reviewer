---
id: "01a1cb45-fcde-75b8-b46f-a17b6db1c36b"
title: "ISSUE-020: Language-neutral complexity in the risk score"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "risk"
  - "analysis"
  - "closed"
created: "2026-11-11T09:42:03-06:00"
timestamp: "2026-11-11T09:42:03-06:00"
description: "The risk score's complexity term counts decision points only in Go; estimate them per line for C-family languages (C#, Java, TypeScript/JavaScript, …), Python and Ruby so non-Go diffs are not under-scored."
---

# ISSUE-020: Language-neutral complexity in the risk score

**Status:** closed · **Raised:** 2026-11-11 from the C# and TypeScript UATs ([ISSUE-016](/issues/issue-016-cross-tool-uat-on-c-with-live-claude-sonnet-5-5-and-opus-5-5--01a1caee2585.md), [ISSUE-018](/issues/issue-018-prove-sarif-lint-input-with-real-eslint-typescript-javascript-output--01a1cb1c9e84.md)) · **Touches:** [ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md), `pkg/risk` ([ISSUE-015](/issues/issue-015-public-llm-free-risk-api-pkg-risk-for-mrman--01a1ca2a2474.md))

## Problem
`decisionPoints` (`internal/analysis/risk.go`) uses Go's scanner and returns 0 for any other file. As a result, the seeded C# change (9 planted defects, 87 added lines) and the TypeScript change (10 defects) both scored `complexity_delta: 0` on every file. Their `base_risk_score` (49 and 46) measured churn only, while a Go change of the same shape would score higher. Auto-approval and garmr thresholds therefore treat non-Go code as simpler than it is.

## Design
- **Same metric for every language:** McCabe-style decision points added minus removed per changed line. The weight (×5) and the "removing complexity never lowers the score" rule (ADR-0008) stay.
- **Go is unchanged:** it keeps the exact `go/scanner` count (`if`, `for`, `case`, `&&`, `||`).
- **Other languages:** a small per-line lexer with no new dependency. It removes string literals and comments, then counts whole-word decision keywords and operators:

| family | extensions | decision points |
|---|---|---|
| C-family | .c .h .cc .cpp .cxx .hpp .cs .java .js .jsx .mjs .cjs .ts .tsx .mts .cts .kt .kts .swift .rs .php .scala .dart .groovy | `if` `for` `foreach` `while` `case` `catch` `guard` `&&` `\|\|` |
| Python | .py .pyi | `if` `elif` `for` `while` `except` `and` `or`, and `case` when it starts the line |
| Ruby | .rb | `if` `elsif` `unless` `while` `until` `for` `when` `rescue` `and` `or` `&&` `\|\|` |

- **Not counted:** the ternary `?:`, `??` and pattern arms (Rust `match`, Kotlin `when` arms). `?` can't be told apart from nullable types, `?.` and generics on one line. Anything else (YAML, Markdown, SQL, …) stays 0.
- **Known imprecision:** the estimate works line by line, so a keyword inside a string or comment that spans lines (a docstring, block comment or template literal) can count. Lines that start a `/*` block or continue one with `*` are skipped.

## Compatibility
`base_risk_score` rises for non-Go diffs. mrman's staleness check compares a report's `base_risk_score` with its own `pkg/risk` assessment. If a report comes from a Vizzini with this change and mrman pins an older Vizzini (v0.1.0 pins `04ea0aa`), every non-Go report will show **STALE**. Shipping this therefore needs a coordinated mrman release that bumps Vizzini.

## Acceptance criteria
- [x] Table tests per family: keywords counted; the same words inside strings, comments and identifiers (`iffy`, `orders`, `format`) not counted; `else if` counts once; Rust lifetimes (`'a`) and JS template strings don't break the lexer; Python `case` only at the start of a line
- [x] The Go count is unchanged (existing tests)
- [x] The existing "non-Go files have no complexity" test becomes the new spec: Python counts, unsupported languages stay 0
- [x] `pkg/risk` and a review report agree for C# and TypeScript diffs (`TestReview_BaseRiskMatchesPkgRisk`-style)
- [x] ADR recording the rule, as an amendment to ADR-0008; risk guide and `FileRisk` docs updated
- [x] vizzini-uat stub run: `complexity_delta` > 0 on the C# and TypeScript fixtures
- [x] Follow-up noted: mrman release bumping Vizzini, so its staleness check agrees

## Resolution (2026-11-11)
Implemented in `internal/analysis/decision.go` ([ADR-0013](/decisions/adr-0013-estimate-decision-points-per-line-for-c-family-python-and-ruby--01a1cb47f7a8.md)). Go keeps its exact scanner count, the other families use the line lexer, and the rules are in the risk guide's table.

**Tests:**
- `TestDecisionPoints` and `TestDecisionPoints_UnterminatedLiterals`: 50+ lines across Go, C#, Java, TypeScript, JavaScript, Rust, Swift, PHP, Python, Ruby, YAML, Markdown, SQL and Makefile.
- `TestReview_BaseRiskMatchesPkgRisk_CSharpAndTypeScript`.
- The old "non-Go files have no complexity" test now asserts the new spec (Python 3, YAML 0); it was not deleted.

**vizzini-uat stub run** (Vizzini `bc9104a`, 20/20 hard checks):

| fixture | base_risk_score before → after | files with branching |
|---|---|---|
| csharp-seeded | 49 → 55 | Paging.cs 2 (`for`, `&&`), ImportService.cs 1 (`while`), RateService.cs 1 (`if`) |
| ts-seeded | 46 → 57 | users.ts 4 (`if`, `for`, `if`, `catch`), config.ts 3 (`for`, `if`, `&&`) |

**mrman follow-up:** mrman v0.1.0's full `internal/...` test suite passes unchanged against this Vizzini (via a scratch go.work). What remains is a `go.mod` bump and a patch release (v0.1.1), so mrman's staleness check matches reports from this version.
