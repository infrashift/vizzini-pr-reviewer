---
id: "01a1ca09-b067-7384-92bf-6546328afceb"
title: "ADR-0010: JSON reports carry an apiVersion/kind envelope; garmr consumes them"
type: "ADR"
tags:
  - "adr"
  - "policy"
  - "garmr"
  - "api"
created: "2026-11-11T03:56:35-06:00"
timestamp: "2026-11-11T03:56:35-06:00"
description: "pr-review JSON is a schema-validated report with apiVersion vizzini.infrashift.io/v1 and a kind; org policy runs in garmr over that contract, the built-in gate stays the offline default."
---

# ADR-0010: JSON reports carry an apiVersion/kind envelope; garmr consumes them

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-011](/issues/issue-011-integrate-the-policy-gate-with-garmr--01a1c9e416ce.md), approved by the user

## Context
[garmr](https://github.com/infrashift/garmr) is infrashift's CUE policy agent. It offers deny/warn/audit enforcement, namespaces, exceptions and an audit log, and it consumes JSON. It targets inputs by `apiVersion` group and `kind`. Vizzini's built-in `--fail-on` gate (ADR-0008's sibling, Milestone 4.1) covers one rule. Organization-level merge policy belongs in a policy engine, but Vizzini must keep working offline in microVMs where no garmr server is reachable.

## Decision
- **Integrate through a data contract, not a dependency.** garmr stays a separate binary or server, and Vizzini's pinned dependency list is unchanged.
- **Envelope:** every `--format json` output is a *report* that starts with `apiVersion: "vizzini.infrashift.io/v1"` and `kind` (`Review`, `PRDescription`, `Suggestions`, `CommandResult` or `ImplementationPlan`), followed by the result's own fields in struct order. `#Report`, `#ReviewReport` and `#FileRisk` in `schema/review.cue` define it. `#ReviewReport` is closed and field-by-field, so policy authors can rely on every path.
- **Self-validation:** `reviewer.MarshalReport` validates each report against its schema *before* printing, so a corrupt result (risk 150, an unknown severity, nil findings) is an error, never output. mrman can use the same function to emit identical reports.
- **Improve JSON** becomes `{apiVersion, kind: "Suggestions", suggestions: [...]}`. Previously it was an untyped map with the same `suggestions` key.
- **Policy pack:** `policies/garmr/vizzini/` (package `vizzini`, namespace `vizzini`), targeting `apiGroup: vizzini.infrashift.io, kind: Review`.
  - `review-gate` (deny): VZ-001 no Critical or Major findings; VZ-002 no Architecture findings (a violated design decision blocks); VZ-003 nothing skipped.
  - `review-advisory` (warn): VZA-001 risk ≤ 70; VZA-002 no sensitive paths (weight < 2.5; an absent `risk_files` passes); VZA-003 at least one chunk reviewed.
  - `garmr test` suites ship with the pack.
- **CI recipe:** `pr-review --fail-on none --format json > review.json`, then `garmr eval --input review.json -n vizzini`. garmr exits 1 for deny and 2 when evaluation can't run; Vizzini's own gate keeps exit 2 for its gate. They run as separate steps, so the codes don't collide.
- **The built-in `#PolicyGate` stays the default,** for offline or microVM use and for anyone without garmr.

## Consequences
- Policies can be tuned by editing plain CUE, centrally, with an audit trail, without rebuilding Vizzini.
- Report fields are now a public contract: renaming one is a breaking change for policies and needs a new `apiVersion`.
- The real-server check found that a `forEach` over an absent list fails in garmr. Rules over `omitempty` fields must allow absence (VZA-002 does).

## Alternatives considered
- **Embed garmr's evaluator as a Go library.** Rejected: it brings garmr's dependency tree into Vizzini and couples release cycles.
- **Wrap reports as `{apiVersion, kind, spec: {...}}`.** Rejected: flat fields keep policy paths short (`findings`, not `spec.findings`) and leave existing consumers' keys unchanged.
