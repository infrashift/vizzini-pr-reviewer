---
id: "01a1cba3-cacc-75d4-94c0-5bb2f8fbf1b7"
title: "ISSUE-025: Usage and cost attribution per request, review run and PR lifecycle"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "cost"
  - "observability"
  - "litellm"
  - "closed"
created: "2026-11-11T11:24:31-06:00"
timestamp: "2026-11-11T11:24:31-06:00"
description: "Attribute LLM usage and dollar cost to who asked (human), what asked (agent/tool), why (mode, PR, iteration) and how (model, effort, retries/repairs), correlated per request, per review run and across a PR's lifecycle, building on LiteLLM's spend logs rather than duplicating them."
---

# ISSUE-025: Usage and cost attribution per request, review run and PR lifecycle

**Status:** closed (design agreed 2026-11-11) · **Raised:** 2026-11-11 · **Builds on:** [ADR-0004](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md) (LiteLLM transport)

## Goal
Answer "what is AI review costing us, and for what?":
- per request;
- per review run, one `pr-review` invocation with its chunks, modes, retries and repairs;
- per PR across its lifecycle, every run against every head SHA;

and roll up by repository, team, requester, agent and model.

## Principles
- **Don't rebuild token accounting.** LiteLLM with Postgres already writes one row per request to `LiteLLM_SpendLogs` (request ID, model, key, end user, tokens, spend, timestamps, metadata and tags) and enforces budgets per virtual key, team or user. Vizzini adds the **business context**: which PR, run, mode, chunk and attempt, and who or what asked.
- **The library stays stateless.** `pkg/reviewer` gains no database driver. It emits context on each request and a usage summary in its report, and storage or joining happens outside, in LiteLLM's Postgres or a small collector.

## Design
- **Correlation IDs:**
  - **run_id:** a UUIDv7 per Vizzini invocation.
  - **pr_key:** `forge:host/org/project/repo#number`.
  - **iteration:** the head SHA.
  - **request_seq:** chunk, attempt and repair number.

  They follow W3C trace-context so they can join OpenTelemetry traces later.
- **Per-request metadata:** sent through LiteLLM's request `metadata`/tags (and `user` for the human), carrying:
  - mode (review, describe, improve, plan, command) and chunk i/n;
  - attempt and repair index, so wasted repair tokens are visible;
  - knowledge and lint and context sizes;
  - requested model and effort;
  - requester identity (forge user from mrman, or CI actor);
  - agent identity (`mrman/v0.1.1`, `vizzini-ci`, `claude-code`, …).
- **Report `usage` block** (closed schema): per-mode and total input and output tokens, cache-read and cache-write tokens, request count, retries, repairs, and estimated cost when the proxy reports it (LiteLLM returns a response cost). It also carries `run_id` and `pr_key`, so a report links to the spend rows.
- **Lifecycle view:** a Postgres view joining spend logs on `metadata.pr_key` / `run_id`. The open question is whether a small `vizzini_runs` table, written by a collector (or by mrman / the CI entry point), records run-level facts LiteLLM doesn't have: findings count, gate decision, outcome.
- **Value, not just cost:** cost per finding, and with mrman/ISSUE-024 feedback, cost per *accepted* finding.

## Caveats found already
- **Pricing:** LiteLLM v1.99.4 doesn't know the 5.5 models (it fell back to a default `max_tokens`). Unless the proxy config sets per-token prices (`model_info`), spend may be recorded as 0. The UAT config must set them.
- **Identity is self-reported metadata.** Use LiteLLM virtual keys per team or service for enforceable chargeback, and treat requester and agent metadata as analytics.
- **Privacy:** don't store prompts or diffs in spend logs by default. Requester identity is personal data, so define retention (LiteLLM supports spend-log retention limits).
- **Effort:** Vizzini doesn't set an effort or thinking budget today. Recording "effort" requires plumbing it through the request first.

## Decision log
- **2026-11-11:** the user agreed the suggested implementation:
  - LiteLLM (Postgres) remains the per-request ledger, and Vizzini adds business context as request metadata.
  - W3C trace-context correlation IDs (`run_id`, `pr_key`, iteration).
  - A `usage` block in the report.
  - The library stays stateless, and run-level facts live outside it.
  - Value metrics (cost per accepted finding, repair waste) and the caveats on pricing, identity, privacy and effort.

  The open question remains where run facts live (a `vizzini_runs` table written by a collector, mrman or CI).

## Acceptance criteria
- [x] ADR on correlation IDs, metadata keys and where run facts live (decide the open question above)
- [x] Vizzini sends per-request metadata through LiteLLM, verified in LiteLLM's spend log rows in vizzini-lab (proxy with Postgres)
- [x] Report `usage` block, populated from responses
- [x] Prices configured for the 5.5 models in the UAT proxy; a run's cost is reported per run and per PR
- [x] A rollup query or view per PR lifecycle, documented

## Resolution (2026-11-11)
Implemented as [ADR-0015](/decisions/adr-0015-cost-attribution-through-litellm-spend-logs-correlated-by-run-and-pull-request--01a1cbc7d120.md). Vizzini: `12beaae` (attribution, meter, usage, CLI) and `3843b53` (LiteLLM headers); views in `deploy/litellm/views.sql`; guide at Guides → Cost Attribution. vizzini-lab runs LiteLLM with Postgres 17 in a podman pod, with prices for the 5.5 models.

**Found on the way:** LiteLLM v1.99.4 persists custom metadata only from the `x-litellm-spend-logs-metadata` header. The body `metadata` was silently dropped. A live probe caught it and the fix is tested.

**Live run 20261111T175706Z** (both fixtures, both models, no forge, 72/72):
- Every review's report `cost_usd` equals LiteLLM's recorded spend for that run (e.g. $0.05025, $0.07652, $0.038854, $0.057708).
- Cost per finding was $0.0035–0.0070.
- Per-PR lifecycle (C# fixture, 8 runs): $0.366, split review 35%, improve 31%, plan 22%, describe 12%.

**Follow-ups:** all done on 2026-11-11 (see below).

## Follow-ups closed (2026-11-11)
- **Usage in every report kind** (Vizzini `cc3f080`): every engine call hands a `RunInfo` (attribution, mode, usage) to a `RunRecorder` on the context. `MarshalReportWithRun` adds the run ID, attribution and `usage` to describe, improve, plan and command reports, and `#Report` types them. The CLI does this for every `--format json` report.
- **Real PR key and requester:**
  - **vizzini-lab** opens the ADO PR *before* the reviews, so every request carries `ado:dev.azure.com/<org>/<project>/<repo>#<id>`, and the requester is the PAT owner's ADO identity (`uat/ado.sh whoami`). The guide has Azure Pipelines and GitHub Actions examples.
  - **mrman** runs no models by design (it reads reports), so there is nothing for it to attribute.
- **Settling wait:** every report now counts its requests, so `uat/costs.sh` (now a standalone, rerunnable script) waits for exactly that many rows.
- **Harness bug found:** the forge leg *exported* `XDG_DATA_HOME` for mrman, which moved rootless podman's storage for the rest of the run. Fixture builds silently re-pulled images, and the cost step could not reach the database. It is now scoped to mrman's own calls.

**Live run 20261111T181101Z** (TypeScript, Sonnet, forge leg, 23/23): ADO PR #12. All four modes are attributed per run (describe $0.016, improve $0.039, plan $0.026, review $0.035 at $0.0035 per finding), and the PR lifecycle total is $0.116. LiteLLM recorded 4 of 4 requests.
