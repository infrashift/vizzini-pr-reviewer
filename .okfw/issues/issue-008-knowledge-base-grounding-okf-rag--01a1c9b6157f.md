---
id: "01a1c9b6-157f-76c0-90e9-3519d6f983b2"
title: "ISSUE-008: Knowledge base grounding (OKF RAG)"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "okf"
  - "milestone-4-5"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:31:06-06:00"
description: "Load OKF index.md and frontmatter, match diff paths to relevant knowledge, and require citations on architectural findings."
---

# ISSUE-008: Knowledge base grounding (OKF RAG)

**Status:** closed · **Phase:** 4 · **Milestone:** 4.5 · **PLAN.md:** Milestone 4.5: Knowledge Base Grounding (OKF RAG)

## Description
Cross-reference an OKF Markdown directory to flag architectural violations and cite specific design decisions.

## Acceptance Criteria
- [x] Parse progressive-disclosure `index.md` files and YAML frontmatter (`tags`, `domain`, `paths`)
- [x] Match diff file paths and package names against frontmatter rules; load only relevant sections
- [x] `#Finding` citations include the matched OKF document ID for architectural violations
- [x] Fixture test with a mock OKF directory (`index.md` + 2 domain docs) verifies selective loading

## Notes
- **Fixture:** build it under `testdata/okf/` with `okfw` (`okfw init` in a temp git repo, or a hand-copied bundle checked with `okfw validate --okf`). The fixture must be OKF-conformant.
- This repo's own `.okfw/` is a working example of okfw's frontmatter shape: `id`, `title`, `type`, `tags`, `description`, plus custom keys under `x:`. Note that okfw's strict schema has **no** top-level `domain` or `paths` keys; those would live under `x:`. Reconcile this with the PLAN.md frontmatter fields before implementing.
- Related tool: https://github.com/infrashift/okf-wiki/

## Resolution
Done 2026-11-11. Decisions: [ADR-0009](/decisions/adr-0009-architecture-findings-must-cite-provided-knowledge-documents--01a1c9f25bc8.md).
- **`internal/okf/grounder.go`:** `Load(fs.FS)` follows progressive disclosure (the root `index.md`, then listed concepts and subdirectory indexes, with a scan fallback). It parses frontmatter with CUE's YAML encoder and reads `id`, `title`, `description` and `tags`, plus `paths`/`domain` from `x.*` or the top level (a string or a list). `TargetsFor` reads Go package clauses from hunk text. `Match` ranks documents: paths > domain > tags.
- **The frontmatter question from Notes, resolved:** okfw's strict schema has no top-level `domain`/`paths`, so the grounder reads `x.paths`/`x.domain` (okfw-compatible) and also accepts top-level keys for other OKF producers.
- **Schema:** the `Architecture` category, plus `citations` with a CUE conditional that requires citations for Architecture.
- **Engine:** `ReviewOptions.Knowledge fs.FS` and `Config.MaxKnowledgeTokens`. Selection and budget are per chunk. A `<knowledge>` block in `review.user` holds `<doc id title>` entries. Citations are anchored to the provided IDs through the repair loop. `ReviewOutput.KnowledgeDocs` lists what was used, and Markdown shows `Cites:` and the consulted documents.
- **CLI:** `--knowledge DIR` (review mode only; must be a directory).
- **Fixture:** `internal/okf/testdata/okf`, which passes `okfw validate --okf`: a root index, a `decisions/` index with two ADRs (one using `x:` keys, one top-level, plus a dangling link), an index-less `notes/` directory, a conventions doc, and an unlisted orphan that must never load.
- **Tests:** progressive disclosure (orphan excluded, dangling link tolerated), frontmatter variants, selective matching (store-only, api-only, both, domain list, docs-only gives none), package-name and tag matching, package clause extraction, edge cases (no index, CRLF, dotfiles, `log.md`, malformed YAML, bad glob, escaping links), and loading this repo's own `.okfw`. Engine tests: documents in the prompt, per-chunk isolation, repairs for an uncited, unknown or knowledge-less citation, budget truncation, and a malformed bundle making no request. Plus the CUE citation rule, the template and the CLI flag.

## Related
- [ADR-0001: Adopt an OKF wiki (okfw) for project knowledge](/decisions/adr-0001-adopt-an-okf-wiki-okfw-for-project-knowledge--01a1c9b61524.md)
- [Review output schema](/specs/review-output-schema--01a1c9b61543.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
