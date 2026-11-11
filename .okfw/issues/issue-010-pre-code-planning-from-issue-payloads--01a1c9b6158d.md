---
id: "01a1c9b6-158d-7963-8118-844321e157f4"
title: "ISSUE-010: Pre-code planning from issue payloads"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "planner"
  - "milestone-4-7"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Turn an IssuePayload plus repo tree and OKF index into a CUE-validated #ImplementationPlan for coding agents."
---

# ISSUE-010: Pre-code planning from issue payloads

**Status:** open · **Phase:** 4 · **Milestone:** 4.7 · **PLAN.md:** Milestone 4.7: Pre-Code Planning (Issue Tracker Sync)

## Description
Analyze an issue ticket and generate a structured implementation plan before any code is written.

## Acceptance Criteria
- [ ] Accept `IssuePayload` (title, body, labels) + repo file-tree summary + OKF `index.md`
- [ ] Output a CUE-validated `#ImplementationPlan` (affected files, phased steps, verification commands)
- [ ] Unit test validates `#ImplementationPlan` unification against a mocked issue payload

## Notes
- The output shape should be directly usable by coding agents. This wiki's issue format is one candidate rendering target.

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [ADR-0001: Adopt an OKF wiki (okfw) for project knowledge](/decisions/adr-0001-adopt-an-okf-wiki-okfw-for-project-knowledge--01a1c9b61524.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
- Depends on: [ISSUE-008: Knowledge base grounding (OKF RAG)](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md)
