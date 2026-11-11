---
id: "01a1c9b6-153c-7f34-87fd-66a5131def65"
title: "ADR-0005: Stateless library-first API with minimal abstractions"
type: "ADR"
tags:
  - "adr"
  - "api"
  - "architecture"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "pkg/reviewer is a stateless Go API embedded by mrman; the CLI is a thin wrapper; no unrequested interfaces or dependencies."
---

# ADR-0005: Stateless library-first API with minimal abstractions

**Status:** Accepted
**Date:** 2026-11-11
**Source:** `CLAUDE.md`, `PLAN.md`

## Context
The main consumer is the `mrman` TUI (bubbletea, bubbles, lipgloss), which embeds Vizzini in-process. The second consumer is the `pr-review` CLI running in isolated containers or microVMs.

## Decision
- The public API is `pkg/reviewer`'s `Engine` with `Describe`, `Review` and `Improve`. Each takes a `context.Context` and an `io.Reader` patch.
- No persistent state, no globals, no hidden config files. Callers pass everything in.
- `cmd/pr-review` is a thin flag-parsing wrapper over `pkg/reviewer`.
- Minimal abstractions: no unrequested interfaces or factory patterns, and dependencies are limited to the pinned list in `CLAUDE.md`.

## Consequences
- Easy to embed, test and run concurrently.
- Caching, thread history and persistence are the caller's (mrman's) responsibility.
- Adding a dependency or an abstraction layer needs a new ADR.

## Alternatives considered
- **A long-running review service.** Rejected: it adds state and deployment surface that neither consumer needs.
