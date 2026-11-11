---
id: "01a1c9b6-1563-7c4b-abb4-c0af5c50bd81"
title: "ISSUE-004: Triage and severity policy gate"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "policy"
  - "milestone-4-1"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:10:32-06:00"
description: "CUE #PolicyGate with fail_on severities; pr-review exits 2 when blocking findings exist."
---

# ISSUE-004: Triage and severity policy gate

**Status:** closed · **Phase:** 4 · **Milestone:** 4.1 · **PLAN.md:** Milestone 4.1: Triage & Severity Policy Gate

## Description
Tag findings with the CUE-enforced severity enum so blocking policies trigger only on high-risk issues.

## Acceptance Criteria
- [x] `#PolicyGate` in `schema/review.cue` with `fail_on: [...#Severity]` (default `["Critical", "Major"]`)
- [x] `internal/schema/policy.go` evaluates the gate
- [x] `cmd/pr-review` exits `2` on matching findings and `0` when only Minor/Trivial findings exist
- [x] Unit tests cover exit codes across all severity permutations

## Resolution
Done 2026-11-11.
- **Schema:** `#PolicyGate: { fail_on: [...#Severity] | *["Critical", "Major"] }`. The default lives in CUE, not Go. `Validator.Policy(failOn)` marshals `{}` (nil → schema default) or `{"fail_on": [...]}`, then unifies and decodes. An unknown severity fails with `ErrInvalid` and the per-branch conflicts.
- **Evaluation:** `PolicyGate.Blocks(sev)` and `Blocking(findings)`. `pkg/reviewer` exports `PolicyGate` and `Engine.Policy`, so mrman can apply the same gate to findings it shows.
- **CLI:** `--fail-on` takes comma-separated severities, or `none` (an empty value also blocks nothing). It is review mode only, and setting it with describe or improve is a usage error. The gate is validated before the patch is read, so a typo costs no tokens. The review output is always printed in full. When it blocks, stderr gets `policy gate failed: N finding(s) at Critical, Major` and the exit code is `2`. Exit codes are now 0 ok, 1 error, 2 gate.
- **Tests:** `TestPolicy_AllSeverityPermutations` covers all 16 gate subsets × 16 finding subsets. There are tests for the default (Critical/Major block, Minor/Trivial pass) and for unknown-severity rejection. CLI exit-code tests cover each severity under the default gate, custom gates, `none`/empty, whitespace in the list, an invalid gate making zero LLM requests, and `--fail-on` outside review mode. The Phase 3 CLI tests now expect exit 2 for their Critical mock review, which is the spec'd default behavior; the output assertions are unchanged.

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
