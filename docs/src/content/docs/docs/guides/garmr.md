---
title: Policy with garmr
description: Let garmr, infrashift's CUE policy agent, make merge decisions from Vizzini's JSON reports.
---

Vizzini's built-in `--fail-on` gate covers one rule and works anywhere,
including offline. For organization-wide merge policy (deny, warn and audit
actions, namespaces, exceptions, and an audit trail), hand Vizzini's report to
[garmr](https://github.com/infrashift/garmr).

## The report contract

Every `--format json` output is a report that starts with an envelope:

```json
{
  "apiVersion": "vizzini.infrashift.io/v1",
  "kind": "Review",
  "summary": "…",
  "risk_score": 42,
  "base_risk_score": 42,
  "model_risk_score": 20,
  "risk_files": [{ "path": "internal/auth/login.go", "weight": 3, "changed": 3, "complexity_delta": 0, "points": 9 }],
  "auto_approve": false,
  "findings": [ … ],
  "chunks": 1
}
```

`#ReviewReport` in `schema/review.cue` defines every field and is closed.
Vizzini validates each report against it before printing, so a policy can rely
on the paths it reads. Other modes emit `kind` `PRDescription`, `Suggestions`,
`CommandResult`, `ImplementationPlan` or, for `--mode risk`,
`RiskAssessment` (closed, `#RiskAssessmentReport`).

## The policy pack

`policies/garmr/vizzini/` targets `apiGroup: vizzini.infrashift.io, kind: Review`:

| Policy | Action | Rule | Fails when |
|---|---|---|---|
| `review-gate` | deny | VZ-001 | any finding is Critical or Major |
| | | VZ-002 | any finding is Architecture (a violated design decision) |
| | | VZ-003 | hunks were skipped for size |
| `review-advisory` | warn | VZA-001 | `risk_score` > 70 |
| | | VZA-002 | a touched file's risk weight ≥ 2.5 (auth, security, crypto, schema, migrations) |
| | | VZA-003 | nothing was reviewable |

Evaluating the `vizzini` namespace gives **deny** if a gate rule fails,
**warn** if only advisory rules fail, and **allow** otherwise. Copy the pack
and tune it; severities and thresholds are plain CUE.

## In CI

```sh
# once: a garmr server with the pack loaded
GARMR_HTTP_ADDR=:8080 garmr-server --policy-dir policies/garmr

# per change: let garmr decide, so Vizzini's own gate is off
git diff origin/main...HEAD | pr-review --mode review --fail-on none --format json > review.json
garmr eval --server http://garmr:8080 --input review.json -n vizzini   # exit 1 = deny
```

## Checking the pack

```sh
garmr validate policies/garmr/vizzini
garmr test policies/garmr/vizzini/review.cue policies/garmr/vizzini/review-gate_test.cue
make e2e-garmr   # real pr-review reports -> real garmr-server: deny / allow / warn
```

`make e2e-garmr` finds `garmr` and `garmr-server` on `PATH` or in
`GARMR_BIN_DIR`, and skips if they aren't there.

## From Go

```go
report, err := reviewer.MarshalReport(out) // same validated JSON as the CLI
```
