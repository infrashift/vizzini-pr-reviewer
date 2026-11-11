---
title: Risk Scoring & Auto-Approve
description: How Vizzini computes a deterministic blast-radius score and decides when a change can be approved automatically.
---

Every review reports three risk numbers and a verdict:

| Field | Meaning |
|---|---|
| `base_risk_score` | Computed from the diff alone, so it's deterministic and repeatable |
| `model_risk_score` | The highest `risk_score` the model gave any chunk |
| `risk_score` | `max(base, model)`: the model can raise the risk, never lower it |
| `auto_approve` | `true` only when every rule below holds |
| `risk_files` | Per-file breakdown of the base score |

## The base score

Each changed file contributes:

```text
points = weight × (changed lines + 5 × max(0, decision points added − removed))
```

Decision points are `if`, `for`, `case`, `&&` and `||` on changed Go lines.
New branching counts five times as much as straight-line code, and removing
complexity never lowers the score. A changed binary file adds 10 points. The
total becomes a score with:

```text
score = round(100 × (1 − e^(−total / scale)))
```

The score rises quickly for the first changes and never exceeds 100.

## Path weights

Weights come from `#RiskPolicy` in `schema/review.cue`. The **first matching
pattern wins**, so tests and docs are listed before the high-risk directories:

| Pattern | Weight |
|---|---|
| `*_test.go` | 0.3 |
| `**/testdata/**` | 0.2 |
| `**/docs/**`, `*.md` | 0.1 |
| `**/auth/**`, `**/security/**`, `**/crypto/**` | 3 |
| `**/schema/**`, `**/migrations/**` | 2.5 |
| anything else (`default_weight`) | 1 |

## When a change is auto-approved

All of these must hold:

1. At least one chunk was reviewed. A lockfile-only change needs a human.
2. No hunk was skipped as oversized.
3. No touched file is **sensitive**, meaning its weight is at least
   `sensitive_weight` (2.5). A one-line auth fix still needs a human.
4. `risk_score ≤ auto_approve_threshold` (20).
5. No `Critical` or `Major` findings.

## Your own policy

Pass a JSON document validated against `#RiskPolicy`. Omitted fields keep
their defaults:

```json
{
  "auto_approve_threshold": 10,
  "weights": [
    { "pattern": "*_test.go", "weight": 0.3 },
    { "pattern": "**/billing/**", "weight": 4 },
    { "pattern": "**/auth/**", "weight": 3 }
  ]
}
```

```sh
pr-review --mode review --risk-policy risk.json --format json
```

From Go, set `reviewer.Config{RiskPolicy: data}`. An invalid policy, such as
a weight above 10, a zero scale or a malformed glob, is rejected when the
engine is created.

A repository keeps its own policy at **`.vizzini/risk.json`**. That's the
conventional location: pass it with `--risk-policy .vizzini/risk.json` in CI,
and embedders such as mrman look for it there.

## Bands

For display, a score falls in one of three bands:

| Band | Scores (defaults) |
|---|---|
| `low` | at or below `auto_approve_threshold` (20) |
| `medium` | between the two thresholds |
| `high` | at or above `high_threshold` (60) |

`high_threshold` must stay above `auto_approve_threshold`. If your policy
raises `auto_approve_threshold` to 60 or more, set `high_threshold` too, or
the policy is rejected.

## Without a model

The base score needs only the diff and the policy, so you can compute it
without a proxy, a model or a tokenizer:

```sh
git diff main...HEAD | pr-review --mode risk
git diff main...HEAD | pr-review --mode risk --risk-policy .vizzini/risk.json --format json
```

The JSON is a `RiskAssessment` report: `{apiVersion, kind, score, band,
files[]}`. Its `score` and `files` equal the `base_risk_score` and
`risk_files` that a review of the same diff would report.

From Go, use `pkg/risk`. It's a dependency leaf: it never links the LLM
client, the tokenizer or the knowledge-base loader, so a program can embed it
just to score changes. See [Embedding in Go](../embedding/#risk-without-a-model).
