---
id: "01a1c9e4-16ce-71a4-8f15-615e7138a0bc"
title: "ISSUE-011: Integrate the policy gate with garmr"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "policy"
  - "garmr"
  - "closed"
created: "2026-11-11T03:15:30-06:00"
timestamp: "2026-11-11T03:56:35-06:00"
description: "Make pr-review JSON a first-class garmr input and ship a vizzini policy pack, keeping the built-in #PolicyGate as the offline default."
---

# ISSUE-011: Integrate the policy gate with garmr

**Status:** closed (approved 2026-11-11) · **Phase:** 4 · **Raised:** 2026-11-11 by the user during Milestone 4.2–4.3

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
- [x] JSON envelope (`apiVersion`, `kind`) in schema and output, with an ADR
- [x] `vizzini` garmr policy pack with `garmr test` fixtures
- [x] Docs guide: CI with garmr
- [x] Optional integration test against a local garmr binary

## Resolution
Done 2026-11-11. Decisions: [ADR-0010](/decisions/adr-0010-json-reports-carry-an-apiversion-kind-envelope-garmr-consumes-them--01a1ca09b067.md).
- **Envelope:** `#Report`, `#ReviewReport` and `#FileRisk` in `schema/review.cue`. `reviewer.MarshalReport` puts `apiVersion`/`kind` first, keeps struct field order, and validates before printing. `TestAPIVersionMatchesSchema` guards the constant. The CLI's JSON output for every mode goes through it, and all pre-existing CLI JSON tests pass unchanged against the stricter contract.
- **Pack:** `policies/garmr/vizzini/review.cue` holds `review-gate` (deny: VZ-001/002/003) and `review-advisory` (warn: VZA-001/002/003), with `garmr test` suites (6 + 5 cases). It passes `garmr validate` and `cue fmt`.
- **Proof** (`make e2e-garmr`, `scripts/e2e-garmr.sh`): validate plus both test suites, then a real `garmr-server` (`GARMR_HTTP_ADDR`, loads `policies/garmr`), then three real `pr-review` reports via the stub model, then `garmr eval -n vizzini`. Results: risky gives **deny** (VZ-001), clean gives **allow**, an auth path gives **warn** (VZA-002). Without garmr binaries the script prints SKIP and exits 0.
- **Found while probing a real server:** a `forEach` over an absent `risk_files` failed VZA-002 with its template unrendered. Fixed with `any: [exists: false, forEach…]` and pinned by a `garmr test` case.
- **Open question resolved:** risk path *weights* stay in Vizzini's `#RiskPolicy` (they feed `base_risk_score`), while *decisions* on risk (thresholds, sensitive paths) can live in garmr (VZA-001/002). Both are tunable.

## Related
- [ADR-0010](/decisions/adr-0010-json-reports-carry-an-apiversion-kind-envelope-garmr-consumes-them--01a1ca09b067.md)
- [ISSUE-004: Triage and severity policy gate](/issues/issue-004-triage-and-severity-policy-gate--01a1c9b61563.md)
- [ISSUE-007: Automated risk scoring and approvals](/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
