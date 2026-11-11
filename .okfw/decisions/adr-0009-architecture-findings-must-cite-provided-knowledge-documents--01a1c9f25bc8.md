---
id: "01a1c9f2-5bc8-7a9e-86c7-e6bcd14a9575"
title: "ADR-0009: Architecture findings must cite provided knowledge documents"
type: "ADR"
tags:
  - "adr"
  - "okf"
  - "schema"
  - "knowledge"
created: "2026-11-11T03:31:06-06:00"
timestamp: "2026-11-11T03:31:06-06:00"
description: "A fifth category, Architecture, requires citations by CUE rule, and every citation must name an OKF document given to that chunk."
---

# ADR-0009: Architecture findings must cite provided knowledge documents

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-008](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md); extends [ADR-0003](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md) and [ADR-0007](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md)

## Context
Milestone 4.5 asks that findings cite the matched OKF document when an architectural constraint is violated. A schema can't tell semantically whether a finding is "architectural", and a model can invent document IDs as easily as line numbers.

## Decision
- **Make "architectural" explicit.** `#Category` gains `"Architecture"`. In `#Finding`, `citations?: [...string & !=""]` plus `if category == "Architecture" { citations: [_, ...] }` means CUE itself rejects an uncited architecture finding.
- **Anchor citations like lines.** Every citation (in any category) must be the ID of a document *given to that chunk*. Otherwise the repair loop sends back the allowed IDs, or says none were provided. This is ADR-0007's anchoring applied to knowledge.
- **Discovery follows OKF progressive disclosure.** Concepts are read via the root `index.md` and the listed subdirectory indexes, and an index-less directory is scanned. Unlisted concepts aren't loaded, dangling listings are tolerated (OKF 6.1), and links can't escape the bundle.
- **Governance keys:** `paths` (globs, `diff.MatchGlob` semantics) and `domain` are read from `x.*`, where okfw's closed schema allows custom keys, and from the top level for other OKF producers. Precedence for selection: paths match > domain names a changed package or directory > tags do. Documents with none of these never match.
- **Budget:** `Config.MaxKnowledgeTokens` (default 2000) per chunk, strongest first. A first document bigger than the budget is truncated at line boundaries and marked `[…truncated]`; later ones are dropped.
- **YAML via `cuelang.org/go/encoding/yaml`**, so the pinned dependency list doesn't grow.

## Consequences
- An Architecture finding always names a real, provided design document, and mrman can link it.
- Without `--knowledge`, the prompt tells the model not to use Architecture. If it does anyway, the uncited finding is repaired by CUE.
- Every place that lists the categories updates automatically: prompts come from Go constants, and `TestEnumsMatchSchema` guards drift.

## Alternatives considered
- **Require citations on every finding when knowledge is present.** Rejected: most findings (a nil dereference) have no design document behind them.
- **A separate YAML library.** Rejected: CUE's encoder is already pinned and pure Go.
