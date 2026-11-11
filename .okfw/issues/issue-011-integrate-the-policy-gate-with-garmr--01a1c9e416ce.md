---
id: "01a1c9e4-16ce-71a4-8f15-615e7138a0bc"
title: "ISSUE-011: Integrate the policy gate with garmr"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "proposed"
  - "phase-4"
  - "policy"
  - "garmr"
created: "2026-11-11T03:15:30-06:00"
timestamp: "2026-11-11T03:15:30-06:00"
description: "Make pr-review JSON a first-class garmr input and ship a vizzini policy pack, keeping the built-in #PolicyGate as the offline default."
---

# ISSUE-011: Integrate the policy gate with garmr

**Status:** open (proposed, awaiting the user's go-ahead) · **Phase:** 4 · **Raised:** 2026-11-11 by the user during Milestone 4.2–4.3

## Description
[garmr](https://github.com/infrashift/garmr) is infrashift's CUE-based policy agent (CLI + REST, deny/warn/audit enforcement, namespaces, exceptions, audit log). It consumes JSON and YAML. Vizzini's `--format json` review output is already schema-validated, so garmr can make organization-level merge decisions on it.

## Proposed design
- **Data contract, not a library dependency.** garmr stays a separate binary or server, and Vizzini keeps its pinned dependency list (CLAUDE.md rule 4). CI runs `pr-review --fail-on none --format json > review.json` and then `garmr eval --input review.json -n vizzini`.
- **The built-in `#PolicyGate` stays the default.** It is needed offline in microVMs where no garmr is reachable.
- **Envelope:** add `apiVersion: "vizzini.infrashift.io/v1"` and `kind: "Review"` (plus `"PRDescription"` / `"Suggestions"`) to the JSON output, so garmr's `target.resources` can match it. Define the envelope in `schema/review.cue`.
- **Policy pack:** `policies/garmr/vizzini/*.cue` reproduces the default gate (`forEach findings, severity notIn [Critical, Major]`, deny) and adds examples: deny Security findings at Major or above, warn when `risk_score > 70`, and deny when `skipped` is non-empty.
- **Testing:** `garmr test` fixtures in the pack, plus a Vizzini integration test that runs `garmr eval` when the binary is on PATH and skips otherwise.

## Open questions
- Should Milestone 4.4's path risk weights live in garmr policies instead of Vizzini's CUE? Decide before 4.4.
- Exit codes: garmr returns 1 for deny and 2 when evaluation did not run, while Vizzini's gate returns 2. That's fine in separate CI steps, but document it.

## Acceptance Criteria
- [ ] JSON envelope (`apiVersion`, `kind`) in schema and output, with an ADR
- [ ] `vizzini` garmr policy pack with `garmr test` fixtures
- [ ] Docs guide: CI with garmr
- [ ] Optional integration test against a local garmr binary

## Related
- [ISSUE-004: Triage and severity policy gate](/issues/issue-004-triage-and-severity-policy-gate--01a1c9b61563.md)
- [ISSUE-007: Automated risk scoring and approvals](/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
