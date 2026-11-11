---
id: "01a1ca32-58e2-7823-af24-f7133759d4f4"
title: "ADR-0011: Public LLM-free pkg/risk; bands in the policy; RiskAssessment report"
type: "ADR"
tags:
  - "adr"
  - "risk"
  - "api"
  - "mrman"
created: "2026-11-11T04:40:59-06:00"
timestamp: "2026-11-11T04:40:59-06:00"
description: "Expose the deterministic risk score as a dependency-leaf pkg/risk with one shared implementation, policy-defined bands (high_threshold), pr-review --mode risk, and ParseReport for embedders like mrman."
---

# ADR-0011: Public LLM-free pkg/risk; bands in the policy; RiskAssessment report

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-015](/issues/issue-015-public-llm-free-risk-api-pkg-risk-for-mrman--01a1ca2a2474.md), from [ISSUE-013](/issues/issue-013-explore-what-mrman-gains-from-consuming-vizzinis-risk-score--01a1ca04b280.md), approved by the user

## Context
mrman wants Vizzini's deterministic risk score in its file tree, MR list and session header, and for agents. It must stay usable without Vizzini and must never call a model on its own. The score ([ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md)) needs only the diff and the policy. But it was reachable only through `pkg/reviewer.Review`, and `pkg/reviewer` links go-openai and tiktoken-go.

## Decision
- **New public package `pkg/risk`, a dependency leaf.** It exposes `LoadPolicy`/`DefaultPolicy`, `AssessPatch`, `Score`, `ScoreFile`, `Combine` and `ParseReport`, plus the `Policy`, `FileChange`, `FileRisk`, `Assessment`, `Band` and `Report` types. A test runs `go list -deps` and fails if the package ever links go-openai, tiktoken-go, `internal/llm`, `internal/chunker`, `internal/okf` or `pkg/reviewer`. CUE stays (embedders validate policies with it). So does `net/http`, which CUE's builtins import anyway.
- **One implementation.** `internal/analysis` holds the scoring. `ScoreRisk` (used by `Review`) is a thin adapter over `ScoreChanges`/`ScoreFile`/`CombineRisk`, and `pkg/risk` aliases the same types. A test proves `risk.AssessPatch` equals a review's `base_risk_score` and `risk_files` under both default and custom policies.
- **Bands live in the policy.** `#RiskPolicy` gains `high_threshold` (default 60, and it must exceed `auto_approve_threshold`). `#RiskBand` is `low` (≤ auto-approve threshold), `medium`, or `high` (≥ high threshold). `RiskPolicy.Band` and `RiskPolicy.IsSensitive` are the single definitions: `autoApprove` now uses `IsSensitive`.
- **`pr-review --mode risk`** prints the assessment with no proxy, model or tokenizer. Its JSON is a new closed report kind, `RiskAssessment` (`#RiskAssessmentReport`: score, band, files), under the ADR-0010 envelope.
- **Reading reviews someone else ran.** `risk.ParseReport` reads the risk fields from either the ADR-0010 `Review` report or a bare `ReviewOutput`. It ignores unknown fields, so newer reports still parse. It rejects other kinds and versions, out-of-range scores, and a `risk_score` that isn't `max(base, model)`. `reviewer.APIVersion` is now defined as `risk.APIVersion`.
- **Policy location convention:** `.vizzini/risk.json` at the repository root (`risk.RepoPolicyPath`).

## Consequences
- An embedder can score every change in a list cheaply and offline, and show bands that match CI's numbers.
- `pkg/risk`'s API and the `RiskAssessment` report are now public contracts. A policy that raises `auto_approve_threshold` to 60 or more must also set `high_threshold`, or it's rejected: an intentional, visible break for such policies.
- `auto_approve` in a parsed report remains a suggestion for embedders. Approval stays a human decision.
