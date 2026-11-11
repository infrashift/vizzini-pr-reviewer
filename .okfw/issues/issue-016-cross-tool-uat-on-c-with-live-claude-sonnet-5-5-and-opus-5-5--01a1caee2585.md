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
  - "open"
created: "2026-11-11T08:06:07-06:00"
timestamp: "2026-11-11T08:06:07-06:00"
description: "Validate Vizzini against C# with live claude-sonnet-5-5 and claude-opus-5-5 through a podman LiteLLM proxy, together with garmr, okf-wiki and mrman on Azure DevOps; the harness lives in the vizzini-uat repository."
---

# ISSUE-016: Cross-tool UAT on C# with live Claude Sonnet 5.5 and Opus 5.5

**Status:** open · **Raised:** 2026-11-11 · **Harness:** the `infrashift/vizzini-uat` repository (sibling checkout `../vizzini-uat`) · **Follows:** [ISSUE-014](/issues/issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md) (Roslyn SARIF, stub model only)

## Why
Every end-to-end proof so far ran against `stubllm`. That proves the plumbing, not how real models behave behind ADR-0004's transport (JSON mode through LiteLLM, the CUE boundary, the repair loop), and it never ran Vizzini together with the tools that consume it.

## Scope
- **Models:** `claude-sonnet-5-5` and `claude-opus-5-5`, through LiteLLM v1.99.4 in podman. The key is a podman secret, and `max_tokens` is explicit because LiteLLM's fallback is 4096.
- **Seeded C# fixture:** a net10.0 library with nine planted defects (SQL injection outside the repository layer, MD5 reset token, hardcoded credential, null dereference, `async void`, `.Result`, off-by-one, undisposed resources, static-dictionary race), a clean decoy, ground truth, and an OKF knowledge base with two applicable ADRs and one irrelevant runbook.
- **Real Roslyn 5.9.0 SARIF** (`CA2100`, `CA5351`, `CS8602`, `CA1822` on changed lines; a pre-existing `CS0168` that must stay out) and grounding with `--knowledge` ([ISSUE-008](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md)).
- **Real-world commits** from serilog and Polly, including one large enough to need several chunks.
- **garmr** local evaluation with `policies/garmr`. **mrman** shows the findings ([ISSUE-017](/issues/issue-017-typed-review-findings-in-pkg-risk-for-mrman--01a1caee2572.md)) and posts them to an Azure DevOps PR after a human grant.

## Acceptance criteria
- [x] Harness committed in vizzini-uat. `make uat-stub` passes every hard check without tokens
- [x] [ISSUE-017](/issues/issue-017-typed-review-findings-in-pkg-risk-for-mrman--01a1caee2572.md): typed findings in `pkg/risk`
- [x] mrman `--vizzini-report`: findings in the TUI and in headless JSON (mrman `feature/vizzini-findings` 836f5b3)
- [ ] `litellm.sh smoke` passes for both models
- [ ] `make uat` passes every hard check for both models. The Sonnet vs Opus comparison and any Vizzini fixes are recorded here

## Known gaps found so far
- The risk score's complexity term is Go-only: every C# file reports `complexity_delta: 0`.
- Knowledge grounding detects only Go `package` clauses, so C# namespaces never match a `domain`. The fixture uses `x.paths` globs instead.
