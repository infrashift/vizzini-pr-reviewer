---
id: "01a1c9b6-1578-77da-bd6f-a5383618572a"
title: "ISSUE-007: Automated risk scoring and approvals"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "risk"
  - "milestone-4-4"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Deterministic 0–100 blast-radius score from lines, complexity delta and CUE path weights; gated AutoApprove."
---

# ISSUE-007: Automated risk scoring and approvals

**Status:** open · **Phase:** 4 · **Milestone:** 4.4 · **PLAN.md:** Milestone 4.4: Automated Risk Scoring & Approvals

## Description
Calculate a blast-radius score from the touched files so low-risk changes can be approved automatically.

## Acceptance Criteria
- [ ] Deterministic base score (0–100): hunk line count + cyclomatic complexity delta + CUE path risk weights
- [ ] Path weights in CUE (`auth/**`, `schema/**` high; `docs/**`, `*_test.go` low)
- [ ] `AutoApprove: true` only when `RiskScore <= threshold` AND zero Critical/Major findings
- [ ] Table-driven tests: docs-only vs security-critical diffs

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-004: Triage and severity policy gate](/issues/issue-004-triage-and-severity-policy-gate--01a1c9b61563.md)
