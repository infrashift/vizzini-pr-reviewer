---
id: "01a1cbc7-d120-7b47-a9f4-b011f4ab4612"
title: "ADR-0015: Cost attribution through LiteLLM spend logs, correlated by run and pull request"
type: "ADR"
tags:
  - "adr"
  - "cost"
  - "observability"
  - "litellm"
created: "2026-11-11T12:03:52-06:00"
timestamp: "2026-11-11T12:03:52-06:00"
description: "Vizzini keeps no ledger: it sends each request's attribution (run, PR, iteration, mode, requester, agent, attempt, repair) as LiteLLM spend-log metadata, tags, end user and W3C traceparent, reports a usage block per review, and ships SQL views that roll costs up per run and per PR."
---

# ADR-0015: Cost attribution through LiteLLM spend logs, correlated by run and pull request

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-025](/issues/issue-025-usage-and-cost-attribution-per-request-review-run-and-pr-lifecycle--01a1cba3cacc.md), design agreed with the user
**Builds on:** [ADR-0004](/decisions/adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md) (LiteLLM transport)

## Context
The organization needs to know what AI review costs and for what: per request, per run, and across a pull request's lifecycle, by requester, agent and model. LiteLLM backed by Postgres already writes one `LiteLLM_SpendLogs` row per request (tokens, spend, model, key, end user) and enforces budgets. It lacks the business context.

## Decision
- **Attribution travels on the context:** `reviewer.WithAttribution(ctx, Attribution{RunID, PRKey, Iteration, Requester, Agent})`, with CLI flags and env defaults. `RunID` defaults to a UUIDv7. Every mode tags its requests; no API signature changed.
- **Per request:**
  - `x-litellm-spend-logs-metadata` (JSON) carries `vizzini.run_id`, `pr_key`, `iteration`, `requester`, `agent`, `mode`, `chunk`, `attempt` and `repair`. Probing a live LiteLLM v1.99.4 showed it persists custom metadata only from this header; body `metadata` was dropped.
  - `x-litellm-tags` carries `vizzini` and `vizzini:<mode>`.
  - The OpenAI `user` is the requester, stored as LiteLLM's end user.
  - The W3C `traceparent` has the run ID as trace ID and a fresh span per request.
  - Body `metadata` is kept, for other OpenAI-compatible proxies.
- **Per run:** the review report's `usage` block (closed `#Usage`) holds requests, retries, repairs, input, output and cached tokens, and `cost_usd` summed from LiteLLM's `x-litellm-response-cost` header. Its values match the spend logs exactly.
- **The library stays stateless:** no database driver in `pkg/reviewer`.
- **Run facts** LiteLLM doesn't have (findings, blocking, gate exit, risk) live in a `vizzini_run_facts` table, filled from the review reports by whatever runs Vizzini (CI, a collector; vizzini-lab's `record_runs.py`). This resolves ISSUE-025's open question: the report is the source of truth, and the table is a copy for joins.
- **`deploy/litellm/views.sql`:** `vizzini_requests` (one row per request), `vizzini_runs` (cost, repair and retry waste, cost per finding) and `vizzini_pr_costs` (lifecycle totals).

## Consequences
- Cost questions become SQL over one database, and budgets stay LiteLLM's job (virtual keys and teams).
- **Prices must be configured** for models LiteLLM doesn't know, or spend is recorded as 0. The UAT proxy sets them for the 5.5 models.
- **Requester and agent are self-reported analytics,** not enforcement. The requester is personal data, so spend-log retention applies.
- **Known gap:** describe, improve and plan reports carry no `usage` block yet. Their costs are visible in the spend logs by `vizzini.mode`.
