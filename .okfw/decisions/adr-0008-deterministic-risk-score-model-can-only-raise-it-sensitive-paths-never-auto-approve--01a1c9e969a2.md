---
id: "01a1c9e9-69a2-7c4b-ab39-736cdce32dc6"
title: "ADR-0008: Deterministic risk score, model can only raise it, sensitive paths never auto-approve"
type: "ADR"
tags:
  - "adr"
  - "risk"
  - "policy"
created: "2026-11-11T03:21:19-06:00"
timestamp: "2026-11-11T03:21:19-06:00"
description: "Risk is max(diff-derived score, model score); auto-approval also requires no sensitive-path files, nothing skipped and something reviewed."
---

# ADR-0008: Deterministic risk score, model can only raise it, sensitive paths never auto-approve

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-007](/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)

## Context
PLAN.md 4.4 asks for a deterministic base score (line count, cyclomatic complexity delta, CUE path weights) and `AutoApprove` when `RiskScore <= threshold` with no Critical or Major findings. Implementing it raised three questions. What happens to the model's own `risk_score`? Is size the right measure for security-critical paths? What about changes that were never really reviewed?

## Decision
- **Score:** each file contributes `weight × (changed lines + 5 × max(0, decision points added − removed))`, plus 10 for a binary file. The total maps to `round(100 × (1 − e^(−total/scale)))`, which rises fast and never exceeds 100. Decision points are counted with `go/scanner` (`if`, `for`, `case`, `&&`, `||`) on Go lines. Other languages contribute churn only. Weights, scale, threshold and sensitivity live in `#RiskPolicy` in `schema/review.cue`. The first matching glob wins, so tests and docs are listed before high-risk directories.
- **`RiskScore = max(BaseRiskScore, ModelRiskScore)`.** The model can raise the risk of a change (it read the code), but never lower it below what the diff shows. Both inputs are reported.
- **`AutoApprove` needs all of:** at least one chunk reviewed, nothing skipped as oversized, no touched file with weight ≥ `sensitive_weight` (default 2.5: auth, security, crypto, schema, migrations), `RiskScore ≤ auto_approve_threshold` (default 20), and no Critical or Major findings.
- **Sensitive paths:** found by the test suite. A 3-line change in `internal/auth/` scored 9 and would otherwise have been auto-approved. Size can't express "this path always needs a human", so it's a separate rule.
- **Lockfile-only changes** (everything excluded, zero chunks) are never auto-approved. A dependency bump is a supply-chain decision, not a no-op.
- **Overrides:** `Config.RiskPolicy` / `pr-review --risk-policy FILE` take JSON validated against `#RiskPolicy`, so omitted fields keep their defaults. Malformed globs are rejected at `New`.

## Consequences
- Auto-approval is conservative and explainable: `risk_files` shows each file's weight, churn, complexity delta and points.
- If [ISSUE-011](/issues/issue-011-integrate-the-policy-gate-with-garmr--01a1c9e416ce.md) (garmr) goes ahead, garmr can consume `base_risk_score`, `risk_files` and `auto_approve` rather than re-deriving them, and org-level overrides can live there.

## Alternatives considered
- **Use the model's score alone.** Rejected: it isn't deterministic, and a model can talk itself into "low risk".
- **Average the base and model scores.** Rejected: a low model score would dilute a high diff score.
- **Only lower the scale.** Rejected: a tiny auth change still scores low on any size-based curve.
