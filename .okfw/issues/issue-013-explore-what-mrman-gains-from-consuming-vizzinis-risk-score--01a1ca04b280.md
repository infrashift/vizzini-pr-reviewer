---
id: "01a1ca04-b280-7cba-844c-b2c3898772c2"
title: "ISSUE-013: Explore what mrman gains from consuming Vizzini's risk score"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "proposed"
  - "discovery"
  - "risk"
  - "mrman"
created: "2026-11-11T03:51:07-06:00"
timestamp: "2026-11-11T03:51:07-06:00"
description: "Discovery: decide which mrman views would use risk_score, base/model split, risk_files and auto_approve, and what LLM-free API that needs."
---

# ISSUE-013: Explore what mrman gains from consuming Vizzini's risk score

**Status:** open (discovery) · **Raised:** 2026-11-11 by the user · **Context:** mrman doesn't read `RiskScore` today, so the 4.4 semantic change (now `max(base, model)`, [ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md)) broke nothing.

## Question
What value would mrman get from consuming Vizzini's risk signals, and which ones are worth wiring in?

## Signals available today (`ReviewOutput`)
| Field | Deterministic? | Needs an LLM call? |
|---|---|---|
| `base_risk_score` (0–100) | yes | no in principle, but today it's only computed inside `Review` |
| `risk_files[]` (path, weight, changed, complexity delta, points) | yes | same |
| `model_risk_score` | no | yes |
| `risk_score` = max(base, model) | partly | yes |
| `auto_approve` | partly | yes (needs findings) |

## Candidate uses in mrman (to evaluate)
1. **MR list triage** (`internal/forge/*/list.go` views): show a risk badge per MR and sort or filter by it, so reviewers start with the riskiest changes. Only the deterministic base score is cheap enough to compute for every MR in a list.
2. **File tree attention** (`internal/app/tree.go`): mark files whose `risk_files` weight is sensitive (≥ `sensitive_weight`), or whose points dominate the change. This would combine with `OrderFilesByDependency` (4.3): read in dependency order, but know where the risk is.
3. **Review-session header:** risk, base vs model, and an `auto_approve` *suggestion*. mrman's human-only submit grant stays authoritative; Vizzini never approves on its own.
4. **Divergence as a signal:** a model score far above the base score means the model saw something the size-based score can't, which is worth surfacing.
5. **Agent collaboration:** an agent following the review over mrman's JSON CLI could receive `risk_files` to prioritize which files it comments on.

## Likely Vizzini-side prerequisite
An **LLM-free API** such as `reviewer.AssessRisk(patch io.Reader, policy) (*Risk, error)` that returns `base_risk_score` and `risk_files` without a review. Uses 1 and 2 need it for speed and cost, since today the base score only comes out of a full `Review`. It's small: `analysis.ScoreRisk` already exists, and only the public wrapper and the policy loading are missing.

## Open questions
- Which of the uses above would reviewers actually use? Validate with the mrman maintainers before building.
- Should mrman show the raw 0–100 number, or bands (low/medium/high) tuned to `auto_approve_threshold`?
- Where does the risk policy live for mrman users? Per repo (`.vizzini/risk.json`?) or in [garmr](/issues/issue-011-integrate-the-policy-gate-with-garmr--01a1c9e416ce.md)?

## Acceptance Criteria
- [x] A short decision (ADR or issue comment) on which uses to pursue, agreed with the mrman side
- [x] If use 1 or 2 is chosen: an LLM-free `AssessRisk` API with tests and docs in Vizzini (tracked in [ISSUE-015](/issues/issue-015-public-llm-free-risk-api-pkg-risk-for-mrman--01a1ca2a2474.md))
- [ ] A follow-up issue in mrman for the UI work

## Decision (2026-11-11, mrman side)
mrman pursues all five uses, opt-in behind `[risk] enabled` (off by default, so plain mrman is unchanged):
- Uses 1, 2 and 5 embed a new LLM-free `pkg/risk` package (see [ISSUE-015](/issues/issue-015-public-llm-free-risk-api-pkg-risk-for-mrman--01a1ca2a2474.md)). It's pure computation, with no model, proxy or network.
- Uses 3 and 4 **ingest a report file** (`pr-review --format json`, or a garmr envelope). mrman never calls a model itself.
- Display: a band plus the number (`● med 42`). Bands come from `auto_approve_threshold` and a new `high_threshold` in `#RiskPolicy`.
- Policy location: `.vizzini/risk.json` at the repo root, then mrman's `[risk] policy`, then the schema defaults.
- `auto_approve` is shown only as a suggestion. mrman's human-only submit grant stays authoritative.

## Related
- [ISSUE-015: Public LLM-free risk API (pkg/risk) for mrman](/issues/issue-015-public-llm-free-risk-api-pkg-risk-for-mrman--01a1ca2a2474.md)
- [ISSUE-007: Automated risk scoring and approvals](/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)
- [ISSUE-006: Semantic dependency walkthroughs](/issues/issue-006-semantic-dependency-walkthroughs--01a1c9b61571.md)
