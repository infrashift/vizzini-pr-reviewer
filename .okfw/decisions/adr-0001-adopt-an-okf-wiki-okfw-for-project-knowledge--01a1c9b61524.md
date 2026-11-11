---
id: "01a1c9b6-1524-7d82-9c0e-888a12aa09eb"
title: "ADR-0001: Adopt an OKF wiki (okfw) for project knowledge"
type: "ADR"
tags:
  - "adr"
  - "process"
  - "okf"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Project knowledge (architecture, specs, ADRs, issues, daily notes) lives in an okfw-managed OKF wiki at .okfw/."
---

# ADR-0001: Adopt an OKF wiki (okfw) for project knowledge

**Status:** Accepted
**Date:** 2026-11-11
**Source:** `CLAUDE.md`, `PLAN.md`

## Context
The project starts with only `CLAUDE.md` and `PLAN.md`. Without a home, the reasons behind decisions (why pure Go, why CUE, why LiteLLM) would end up in chat history and commit messages, where agents can't find them. Vizzini will also *consume* OKF (Milestone 4.5, OKF RAG grounding), so using the format on itself gives real experience with it. Sibling infrashift repos (`tenet-cac`, `cuix`, `garmr-policy-agent`) already use okfw.

## Decision
- An OKF wiki lives at `.okfw/` in the repo root, managed by the `okfw` CLI, and is committed with the code.
- `okfw install --platform claude` provides the Claude Code skill and the managed `CLAUDE.md` block. A project `.mcp.json` registers `okfw mcp`.
- `PLAN.md` remains the phase checklist. Each phase/milestone has a mirror issue in `issues/`, and the two link to each other.
- Conventions: [Wiki conventions](/wiki-conventions--01a1c9b61519.md). Loop: [Development workflow](/development-workflow--01a1c9b6151e.md).

## Consequences
- Decisions and open work can be searched (`okfw search`) and are linked into a graph.
- `PLAN.md` and the issues can drift apart. CLAUDE.md rule 5 and the workflow guide require updating both.
- This wiki is a working reference for the OKF fixture Milestone 4.5 needs.

## Alternatives considered
- **Plain `docs/` markdown.** Rejected: no frontmatter, no typed search, no generated indexes.
- **An external issue tracker only.** Rejected: agents running offline in containers or microVMs can't reach it, and it separates the reasoning from the code.
