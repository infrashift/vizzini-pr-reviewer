---
id: "01a1c9b6-157f-76c0-90e9-3519d6f983b2"
title: "ISSUE-008: Knowledge base grounding (OKF RAG)"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "okf"
  - "milestone-4-5"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Load OKF index.md and frontmatter, match diff paths to relevant knowledge, and require citations on architectural findings."
---

# ISSUE-008: Knowledge base grounding (OKF RAG)

**Status:** open · **Phase:** 4 · **Milestone:** 4.5 · **PLAN.md:** Milestone 4.5: Knowledge Base Grounding (OKF RAG)

## Description
Cross-reference an OKF Markdown directory to flag architectural violations and cite specific design decisions.

## Acceptance Criteria
- [ ] Parse progressive-disclosure `index.md` files and YAML frontmatter (`tags`, `domain`, `paths`)
- [ ] Match diff file paths and package names against frontmatter rules; load only relevant sections
- [ ] `#Finding` citations include the matched OKF document ID for architectural violations
- [ ] Fixture test with a mock OKF directory (`index.md` + 2 domain docs) verifies selective loading

## Notes
- **Fixture:** build it under `testdata/okf/` with `okfw` (`okfw init` in a temp git repo, or a hand-copied bundle checked with `okfw validate --okf`). The fixture must be OKF-conformant.
- This repo's own `.okfw/` is a working example of okfw's frontmatter shape: `id`, `title`, `type`, `tags`, `description`, plus custom keys under `x:`. Note that okfw's strict schema has **no** top-level `domain` or `paths` keys; those would live under `x:`. Reconcile this with the PLAN.md frontmatter fields before implementing.
- Related tool: https://github.com/infrashift/okf-wiki/

## Related
- [ADR-0001: Adopt an OKF wiki (okfw) for project knowledge](/decisions/adr-0001-adopt-an-okf-wiki-okfw-for-project-knowledge--01a1c9b61524.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
