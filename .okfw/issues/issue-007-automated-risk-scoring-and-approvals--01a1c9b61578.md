---
id: "01a1c9b6-1578-77da-bd6f-a5383618572a"
title: "ISSUE-007: Automated risk scoring and approvals"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "risk"
  - "milestone-4-4"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:21:56-06:00"
description: "Deterministic 0–100 blast-radius score from lines, complexity delta and CUE path weights; gated AutoApprove."
---

# ISSUE-007: Automated risk scoring and approvals

**Status:** closed · **Phase:** 4 · **Milestone:** 4.4 · **PLAN.md:** Milestone 4.4: Automated Risk Scoring & Approvals

## Description
Calculate a blast-radius score from the touched files so low-risk changes can be approved automatically.

## Acceptance Criteria
- [x] Deterministic base score (0–100): hunk line count + cyclomatic complexity delta + CUE path risk weights
- [x] Path weights in CUE (`auth/**`, `schema/**` high; `docs/**`, `*_test.go` low)
- [x] `AutoApprove: true` only when `RiskScore <= threshold` AND zero Critical/Major findings
- [x] Table-driven tests: docs-only vs security-critical diffs

## Resolution
Done 2026-11-11. Decisions: [ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md).
- **Schema:** `#RiskWeight` and `#RiskPolicy` (weights with first match winning, `default_weight` 1, `scale` 150, `auto_approve_threshold` 20, `sensitive_weight` 2.5). Defaults: `*_test.go` 0.3, `**/testdata/**` 0.2, `**/docs/**` and `*.md` 0.1, `**/auth|security|crypto/**` 3, `**/schema|migrations/**` 2.5. `Validator.RiskPolicy(json)` fills in omitted fields from the defaults.
- **`internal/analysis/risk.go`:** `ScoreRisk(files, policy)` returns a 0–100 score and a per-file `FileRisk` breakdown (weight, changed, complexity delta, points) sorted by points. Decision points come from a `go/scanner` count on Go lines. Negative complexity deltas don't lower the score. `diff.MatchGlob` is exported so weights share the exclude-glob semantics.
- **Engine:** `ReviewOutput` gains `BaseRiskScore`, `ModelRiskScore`, `RiskFiles` and `AutoApprove`. **`RiskScore` now means max(base, model); in Phase 3 it was the model's maximum.** `Config.RiskPolicy` / `--risk-policy FILE`. Markdown shows `**Risk:** N/100 (diff B, model M)` and an auto-approve line when approved.
- **Beyond the spec, found by tests:** sensitive paths block auto-approval regardless of score, because a 3-line auth change scored 9. Auto-approval also requires something reviewed (lockfile-only changes need a human) and nothing skipped.
- **Tests:** docs-only vs tests-only vs ordinary vs security-critical vs schema score bands and threshold side; exact breakdown arithmetic (`3 × (2 + 5×2) = 36`); first-match precedence; negative complexity; non-Go files; binary files; bounded and monotonic up to 100k lines; schema defaults, overrides and rejections. The engine auto-approve matrix covers clean, Minor, Major, Critical, model-raised risk, policy threshold, sensitive path, the same diff outside auth, sensitivity raised by policy, lockfile-only and skipped. There are CLI JSON fields and `--risk-policy` tests. The Markdown golden was updated for the new risk line.

## Related
- [ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md)
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-004: Triage and severity policy gate](/issues/issue-004-triage-and-severity-policy-gate--01a1c9b61563.md)
