---
id: "01a1cb47-f7a8-744f-bd8b-ebad9f133f97"
title: "ADR-0013: Estimate decision points per line for C-family, Python and Ruby"
type: "ADR"
tags:
  - "adr"
  - "risk"
  - "analysis"
created: "2026-11-11T09:44:13-06:00"
timestamp: "2026-11-11T09:44:13-06:00"
description: "The risk score's complexity term counts McCabe decision points in C-family languages, Python and Ruby with a dependency-free line lexer, keeping Go's exact go/scanner count; other files count 0."
---

# ADR-0013: Estimate decision points per line for C-family, Python and Ruby

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-020](/issues/issue-020-language-neutral-complexity-in-the-risk-score--01a1cb45fcde.md)
**Amends:** [ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md) (deterministic risk score)

## Context
ADR-0008's complexity term counted decision points only in Go, so the C# and TypeScript UATs scored `complexity_delta: 0` on every file. A non-Go change scored on churn alone, below an equally branchy Go change. That understated risk exactly where auto-approval and garmr thresholds read it.

## Decision
- **Same metric for every language:** McCabe decision points added minus removed per changed line, worth 5 changed lines each, never lowering the score.
- **Go** keeps the exact `go/scanner` count.
- **C-family, Python and Ruby** use a per-line lexer in `internal/analysis/decision.go`, with no new dependency. It drops string literals (quotes, escapes, JS template literals; not single quotes in Rust, because of lifetimes) and comments (`//` and `/* */`, or `#`). It skips lines that continue a block comment or are preprocessor lines, then counts whole-word keywords and `&&`/`||`. See the table in the risk guide.
- **Not counted:** the ternary `?:`, `??` and pattern-match arms (ambiguous on one line). Every other language counts 0.
- **Accepted imprecision:** the lexer is line-based, so a keyword inside a docstring, block comment or template literal that spans lines can count. The score is a blast-radius estimate, not a metric to compare exactly across languages.

## Consequences
- `base_risk_score` rises for non-Go diffs with new branching. Policies tuned on non-Go changes may need a new `scale` or thresholds.
- **Version skew shows as stale:** mrman compares a report's `base_risk_score` with its own `pkg/risk` assessment. A report from this Vizzini looks stale to an mrman that pins an older Vizzini (v0.1.0 pins `04ea0aa`) whenever the diff has non-Go branching. mrman must bump Vizzini in its next release.
- **Alternatives rejected:**
  - Tree-sitter or per-language parsers: cgo or heavy dependencies, against ADR-0002 and the pinned-library rule.
  - Asking the model for complexity: no longer deterministic, against ADR-0008.
