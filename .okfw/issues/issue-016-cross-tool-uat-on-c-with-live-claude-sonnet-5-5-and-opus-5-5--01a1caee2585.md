---
id: "01a1caee-2585-7d3e-8cf5-79ad95208f38"
title: "ISSUE-016: Cross-tool UAT on C# with live Claude Sonnet 5.5 and Opus 5.5"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "uat"
  - "csharp"
  - "live"
  - "litellm"
  - "mrman"
  - "garmr"
  - "closed"
created: "2026-11-11T08:06:07-06:00"
timestamp: "2026-11-11T08:06:07-06:00"
description: "Validate Vizzini against C# with live claude-sonnet-5-5 and claude-opus-5-5 through a podman LiteLLM proxy, together with garmr, okf-wiki and mrman on Azure DevOps; the harness lives in the vizzini-lab repository."
---

# ISSUE-016: Cross-tool UAT on C# with live Claude Sonnet 5.5 and Opus 5.5

**Status:** closed · **Raised:** 2026-11-11 · **Harness:** the `infrashift/vizzini-lab` repository (sibling checkout `../vizzini-lab`) · **Follows:** [ISSUE-014](/issues/issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md) (Roslyn SARIF, stub model only)

## Why
Every end-to-end proof so far ran against `stubllm`. That proves the plumbing, not how real models behave behind ADR-0004's transport (JSON mode through LiteLLM, the CUE boundary, the repair loop), and it never ran Vizzini together with the tools that consume it.

## Scope
- **Models:** `claude-sonnet-5-5` and `claude-opus-5-5`, through LiteLLM v1.99.4 in podman. The key is a podman secret, and `max_tokens` is explicit because LiteLLM's fallback is 4096.
- **Seeded C# fixture:** a net10.0 library with nine planted defects (SQL injection outside the repository layer, MD5 reset token, hardcoded credential, null dereference, `async void`, `.Result`, off-by-one, undisposed resources, static-dictionary race), a clean decoy, ground truth, and an OKF knowledge base with two applicable ADRs and one irrelevant runbook.
- **Real Roslyn 5.9.0 SARIF** (`CA2100`, `CA5351`, `CS8602`, `CA1822` on changed lines; a pre-existing `CS0168` that must stay out) and grounding with `--knowledge` ([ISSUE-008](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md)).
- **Real-world commits** from serilog and Polly, including one large enough to need several chunks.
- **garmr** local evaluation with `policies/garmr`. **mrman** shows the findings ([ISSUE-017](/issues/issue-017-typed-review-findings-in-pkg-risk-for-mrman--01a1caee2572.md)) and posts them to an Azure DevOps PR after a human grant.

## Acceptance criteria
- [x] Harness committed in vizzini-lab. `make uat-stub` passes every hard check without tokens
- [x] [ISSUE-017](/issues/issue-017-typed-review-findings-in-pkg-risk-for-mrman--01a1caee2572.md): typed findings in `pkg/risk`
- [x] mrman `--vizzini-report`: findings in the TUI and in headless JSON (mrman `feature/vizzini-findings` 836f5b3)
- [x] `litellm.sh smoke` passes for both models
- [x] `make uat` passes every hard check for both models. The Sonnet vs Opus comparison and any Vizzini fixes are recorded here

## Known gaps found so far
- The risk score's complexity term is Go-only: every C# file reports `complexity_delta: 0`.
- Knowledge grounding detects only Go `package` clauses, so C# namespaces never match a `domain`. The fixture uses `x.paths` globs instead.

## Resolution (2026-11-11, run 20261111T142959Z)
**86/86 hard checks passed** for `claude-sonnet-5-5` and `claude-opus-5-5` through LiteLLM v1.99.4 in podman. The forge leg ran on Azure DevOps (`scratch/vizzini-scratch`, PRs #4 and #5): mrman's headless session carried every finding, a person granted submit in the TUI, and every finding was posted as an inline thread. The full summary is in vizzini-lab `results/2026-11-11-live-sonnet-opus-5-5.md`.

| | Sonnet 5.5 | Opus 5.5 |
|---|---|---|
| Seeded defects caught | 9/9 | 9/9 |
| Category agreement | 100% | 100% |
| Findings (Critical/Major/Minor/Trivial) | 12 (4/6/1/1) | 14 (4/6/3/1) |
| Architecture findings citing the right ADR | 3 | 4 |
| Findings on the clean decoy | 0 | 0 |
| Risk (final/diff/model) | 95/49/95 | 95/49/95 |
| garmr | deny VZ-001, VZ-002, VZA-001 | same |
| Review / describe / improve / plan | 34s / 10s / 26s / 20s | 26s / 8s / 16s / 24s |
| ADO inline threads | 12 | 14 |

- **Real-world commits.** The large serilog commit took 2 chunks with both models. Both independently flagged a real bug: `SelfLogFailureListener` passes an interpolated string to `string.Format` with no arguments, so braces in the message or the exception text throw `FormatException`. Opus reported more Major findings (4 vs 1). The small serilog fix got 0 findings from both models, and the Polly change got 1–2 Minor/Trivial findings.
- **Unmatched findings.** The only finding outside the ground truth was CA1822 (Trivial), which echoes Roslyn.
- **Transport.** The repair loop never gave up. JSON mode through LiteLLM came back as bare JSON for both models.
- **Ground-truth fix.** The fixture now also accepts Architecture for `async void`, `.Result` and disposal, because the fixture's own ADR-0002 governs those rules.

### Follow-ups
- The risk score's complexity term is Go-only: C# files report `complexity_delta: 0`.
- Knowledge grounding matches only Go `package` clauses. C# namespaces need `x.paths` globs.
- Before mrman's branch can merge, Vizzini must be pushed and mrman's `go.mod` bumped.
