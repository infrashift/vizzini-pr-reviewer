---
id: "01a1c9b6-1537-7128-b670-8600edb35eac"
title: "ADR-0004: OpenAI-compatible transport through a LiteLLM proxy"
type: "ADR"
tags:
  - "adr"
  - "llm"
  - "transport"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "LLM calls go through go-openai with a custom BaseURL targeting an internal LiteLLM proxy (or mock endpoints in tests)."
---

# ADR-0004: OpenAI-compatible transport through a LiteLLM proxy

**Status:** Accepted
**Date:** 2026-11-11
**Source:** `CLAUDE.md`, `PLAN.md`

## Context
Model choice, routing, keys and quotas are managed centrally in an internal LiteLLM proxy. Tests need a deterministic mock endpoint, and Vizzini shouldn't hard-code a provider.

## Decision
- `github.com/sashabaranov/go-openai` with a configurable `BaseURL` (CLI flag `--litellm-url`).
- JSON output mode is requested, with retry and backoff on transport errors and schema rejections.
- Prompts are `text/template` files embedded from `internal/llm/templates/` (describe, review, improve, and later the slash commands).
- Tests use `httptest.Server` to mock the proxy.

## Consequences
- The engine doesn't depend on a provider, and switching models is a proxy configuration change.
- JSON-mode support varies by backend behind LiteLLM, so the real guarantee is the CUE boundary in [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md).
- Provider-specific features are available only as far as LiteLLM maps them.

## Alternatives considered
- **Provider-native SDKs** (e.g. the Anthropic Go SDK). Rejected: more dependencies, one code path per provider, and they bypass the central proxy.
