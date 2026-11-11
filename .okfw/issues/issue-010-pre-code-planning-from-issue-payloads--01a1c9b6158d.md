---
id: "01a1c9b6-158d-7963-8118-844321e157f4"
title: "ISSUE-010: Pre-code planning from issue payloads"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "planner"
  - "milestone-4-7"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:38:26-06:00"
description: "Turn an IssuePayload plus repo tree and OKF index into a CUE-validated #ImplementationPlan for coding agents."
---

# ISSUE-010: Pre-code planning from issue payloads

**Status:** closed · **Phase:** 4 · **Milestone:** 4.7 · **PLAN.md:** Milestone 4.7: Pre-Code Planning (Issue Tracker Sync)

## Description
Analyze an issue ticket and generate a structured implementation plan before any code is written.

## Acceptance Criteria
- [x] Accept `IssuePayload` (title, body, labels) + repo file-tree summary + OKF `index.md`
- [x] Output a CUE-validated `#ImplementationPlan` (affected files, phased steps, verification commands)
- [x] Unit test validates `#ImplementationPlan` unification against a mocked issue payload

## Notes
- The output shape should be directly usable by coding agents. This wiki's issue format is one candidate rendering target.

## Resolution
Done 2026-11-11.
- **Schema:** `#FileChangeKind`, `#AffectedFile`, `#PlanStep`, `#PlanPhase` and `#ImplementationPlan` (summary, at least one affected file, at least one phase with at least one step each, at least one non-empty verification command, optional risks and references), with matching Go types and `ValidatePlan`.
- **`pkg/reviewer/planner.go`:** `Engine.Plan(ctx, *PlanRequest{Issue IssuePayload, FileTree []string, OKFIndex string})`. The tree is budgeted to 4000 tokens, with an "N more paths omitted" note; the index is budgeted to 1500 tokens and truncated.
- **Anchoring, the planner's version of ADR-0007:**
  - `modify`/`delete` paths must exist in `FileTree`, and `create` paths must not.
  - Every step file must be in `affected_files`.
  - `references` must be concept paths linked from the OKF index; without an index, there may be no references.
  - Violations go back for repair with precise messages. With no tree, the existence checks are skipped. An empty issue returns `ErrEmptyIssue` before any request.
- **Agent handoff:** `FormatPlan` renders the same shape as PLAN.md: affected files, `## Phase n:` sections with `- [ ]` checkbox steps, a `sh` verification block, risks and references.
- **CLI:** `--mode plan --issue issue.json [--tree tree.txt] [--okf-index .okfw/index.md]`. Stdin is not read in plan mode.
- **Tests:** schema table (7 rejections), acceptance unification against a mocked issue payload with every prompt section checked, five anchoring repairs, no-tree mode, tree budget truncation, empty issue and schema exhaustion, `FormatPlan` golden, CLI plan mode.

## Related
- [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md)
- [ADR-0001: Adopt an OKF wiki (okfw) for project knowledge](/decisions/adr-0001-adopt-an-okf-wiki-okfw-for-project-knowledge--01a1c9b61524.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
- Depends on: [ISSUE-008: Knowledge base grounding (OKF RAG)](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md)
