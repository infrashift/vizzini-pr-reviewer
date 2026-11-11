---
id: "01a1cb3c-a045-7e51-bf6e-abd7c8d2d5ec"
title: "ADR-0012: Fold same-span duplicate findings that cite the same knowledge document"
type: "ADR"
tags:
  - "adr"
  - "review"
  - "llm"
  - "findings"
created: "2026-11-11T09:31:50-06:00"
timestamp: "2026-11-11T09:31:50-06:00"
description: "When one problem is reported both by its nature and as the ADR it violates (same file, overlapping lines, a shared citation, exactly one Architecture), Vizzini folds the pair into one Architecture finding with the higher severity."
---

# ADR-0012: Fold same-span duplicate findings that cite the same knowledge document

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-019](/issues/issue-019-deduplicate-same-span-findings-security-architecture-pairs--01a1cb341ed5.md)
**Amends:** [ADR-0007](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md) (deterministic merge)
**Builds on:** [ADR-0009](/decisions/adr-0009-architecture-findings-must-cite-provided-knowledge-documents--01a1c9f25bc8.md) (Architecture findings must cite)

## Context
In the live TypeScript UAT, Opus 5.5 reported four problems twice each: once by their nature (Critical/Security) and once as the ADR they violate (Major/Architecture). Both halves cited the ADR, on overlapping lines. Their titles often share no words ("hardcoded live payments token" vs "Secret literal violates ADR-0002"). Each duplicate became an extra PR thread through mrman. ADR-0007's merge only sorted findings.

## Decision
- **Prompt:** the knowledge section asks for one finding per problem: a defect that also violates a document is reported once, as Architecture with the citation and the defect's severity.
- **Deterministic fold** in `Engine.Review`, after all chunks are validated and anchored and before the final sort. An Architecture finding folds into a partner when all of these hold:
  - same file;
  - overlapping line ranges;
  - the partner is not Architecture;
  - at least one citation in common.

  A shared citation is the test, not title similarity: it is what ties two findings to one decision, and it keeps a different problem on the same line (say `eval` next to an unsafe return) separate.
- **The merged finding:**
  - is **Architecture** (decided on ISSUE-019), so garmr VZ-002 needs no change;
  - takes the more severe severity;
  - takes the partner's title (the specific problem);
  - has body = partner body + "Violates a documented design decision: " + Architecture body;
  - uses the partner's suggestion, or the Architecture one's if the partner has none;
  - spans both ranges and unions the citations. A span covering two anchored, overlapping ranges still overlaps the diff.
- **One-to-one and order-independent:** findings are put in a total order first, and each Architecture finding folds into its most severe eligible partner. A folded finding takes part in no further folds.
- **Reporting:** the report's `merged_findings` (optional, ≥ 1, in the closed `#ReviewReport`) counts the folded pairs.

## Consequences
- Fewer duplicate threads on PRs. Severity, the policy gate and the risk score are unchanged, because the most severe half is kept.
- A model that splits one problem across two findings *without* a shared citation is not folded. The prompt rule is the remedy for that.
- **Alternatives rejected:**
  - Keeping the Security category plus citations would need VZ-002 to trigger on citations ([ISSUE-011](/issues/issue-011-integrate-the-policy-gate-with-garmr--01a1c9e416ce.md) policy pack).
  - Rejecting such pairs in validation would cost a repair round trip and tie a style rule to the schema boundary.
