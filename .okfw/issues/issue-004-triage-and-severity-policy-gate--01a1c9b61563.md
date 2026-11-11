---
id: "01a1c9b6-1563-7c4b-abb4-c0af5c50bd81"
title: "ISSUE-004: Triage and severity policy gate"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "policy"
  - "milestone-4-1"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "CUE #PolicyGate with fail_on severities; pr-review exits 2 when blocking findings exist."
---

# ISSUE-004: Triage and severity policy gate

**Status:** open · **Phase:** 4 · **Milestone:** 4.1 · **PLAN.md:** Milestone 4.1: Triage & Severity Policy Gate

## Description
Tag findings with the CUE-enforced severity enum so blocking policies trigger only on high-risk issues.

## Acceptance Criteria
- [ ] `#PolicyGate` in `schema/review.cue` with `fail_on: [...#Severity]` (default `["Critical", "Major"]`)
- [ ] `internal/schema/policy.go` evaluates the gate
- [ ] `cmd/pr-review` exits `2` on matching findings and `0` when only Minor/Trivial findings exist
- [ ] Unit tests cover exit codes across all severity permutations

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
