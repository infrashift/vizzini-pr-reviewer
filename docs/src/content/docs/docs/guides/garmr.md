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

garmr evaluates the pack in-process, so a CI job needs only the `garmr`
binary next to `pr-review`. No server, network or sidecar is involved, and it
works the same in an offline container or microVM:

```sh
# Let garmr decide, so Vizzini's own gate is off.
git diff origin/main...HEAD | pr-review --mode review --fail-on none --format json > review.json
garmr eval --policy-dir policies/garmr --input review.json -n vizzini \
  --request-id "$REPO#$PR@$SHA"   # exit 1 = deny, 2 = garmr could not run
```

`--policy-dir` takes a directory or a single `.cue` file, so a repository can
keep its own copy of the pack and tune it. The decision is **allow** or
**warn** (exit 0) or **deny** (exit 1). `--request-id` follows garmr's
`<repo>#<pr>@<sha>` convention, so the decision can be traced back to the
change.

If your organization runs a central garmr server to keep one audit trail,
load the same directory there and point the CLI at it instead. The decision
is identical:

```sh
garmr eval --server https://garmr.example.internal --input review.json -n vizzini
```

### Lists that may be absent

Vizzini leaves out empty optional lists: `risk_files`, `skipped`,
`excluded` and `knowledge_docs` are missing from the JSON when there is
nothing to list. `findings` is always present. A garmr `forEach` over a
missing path **fails** by default, so a rule over an optional list sets
`allowMissing: true` to treat it as empty. VZA-002 does this:

```cue
expr: forEach: {
	path:         "risk_files"
	as:           "r"
	allowMissing: true
	condition: match: {path: "r.weight", lessThan: 2.5}
}
```

The pack therefore needs a garmr (and, for `--server`, a garmr-server) that
knows `allowMissing`. It shipped together with `--policy-dir`, and an older
garmr rejects the field when it loads the pack. To stay on an older garmr,
write the rule as
`any: [{match: {path: "risk_files", exists: false}}, {forEach: …}]`.

## Checking the pack

```sh
garmr validate policies/garmr/vizzini
garmr test policies/garmr/vizzini/review.cue policies/garmr/vizzini/review-gate_test.cue
make e2e-garmr   # real pr-review reports -> garmr eval --policy-dir: deny / allow / warn
```

`make e2e-garmr` finds `garmr` on `PATH` or in `GARMR_BIN_DIR`, and skips if
it isn't there or is too old for `--policy-dir`. It runs four reports through
the pack and checks the decision, the exact rules that fail and garmr's exit
code: SQL built from input gives deny (VZ-001), a clean change gives allow,
an auth path gives warn (VZA-002), and a lock-file-only change gives warn
(VZA-003, with no `risk_files` in the report).

## From Go

```go
report, err := reviewer.MarshalReport(out) // same validated JSON as the CLI
```
