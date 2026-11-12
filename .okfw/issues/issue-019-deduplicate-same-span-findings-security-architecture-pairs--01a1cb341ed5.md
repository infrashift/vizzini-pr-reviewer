---
id: "01a1cb34-1ed5-735b-ba03-e2fa83519875"
title: "ISSUE-019: Deduplicate same-span findings (Security + Architecture pairs)"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "review"
  - "llm"
  - "findings"
  - "closed"
created: "2026-11-11T09:22:33-06:00"
timestamp: "2026-11-11T09:22:33-06:00"
description: "A model sometimes reports one problem twice on the same lines, once by its nature (Security) and once as an ADR violation (Architecture); Vizzini's merge only sorts, so both reach the report, mrman and the PR."
---

# ISSUE-019: Deduplicate same-span findings (Security + Architecture pairs)

**Status:** closed · **Raised:** 2026-11-11, from the live TypeScript UAT ([ISSUE-018](/issues/issue-018-prove-sarif-lint-input-with-real-eslint-typescript-javascript-output--01a1cb1c9e84.md)) · **Touches:** [ADR-0007](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md) (deterministic merge)

## Problem
In vizzini-lab run 20261111T150647Z (`ts-seeded`), Opus 5.5 reported **15 findings for 11 distinct problems**. Four problems were each reported twice on the same lines:

| file:lines | first finding | duplicate |
|---|---|---|
| `src/calc.ts:2` | Critical/Security "Remove eval of user-supplied formula" | Major/Architecture "eval violates ADR-0001" |
| `src/config.ts:1` | Critical/Security "Remove hardcoded live payments token" | Major/Architecture "Secret literal violates ADR-0002" |
| `src/export.ts:4` | Critical/Security "Command injection via execSync string" | Major/Architecture "execSync with command string violates ADR-0001" |
| `src/webhook.ts:3` / `3-7` | Critical/Security "Empty-string fallback for HMAC secret" | Major/Architecture "Secret handling violates ADR-0002" |

On the same fixture, Sonnet 5.5 made one finding per problem: Architecture with the ADR citation where one applied. The C# fixture showed the same tendency, more mildly.

## Why it matters
- **Noise on the PR:** the duplicates became 4 extra Azure DevOps threads through mrman. Reviewers read the same issue twice.
- **No effect on the gate:** the policy gate, `risk_score` and recall were unaffected, because the most severe duplicate already blocks. So this is a quality-of-signal problem, not a correctness one.
- **Policy coupling:** garmr's VZ-002 fails on any Architecture finding ([ISSUE-011](/issues/issue-011-integrate-the-policy-gate-with-garmr--01a1c9e416ce.md)). A dedupe that folds an Architecture duplicate into a Security finding drops that signal unless VZ-002 also keys on `citations`.

## Today's behavior
ADR-0007's merge sorts findings by severity, file, line and title. It never compares them, either within one model answer or across chunks.

## Options
1. **Prompt rule (cheap):** in `review.tmpl`, say "one finding per problem. When a problem also breaks a knowledge document, put the citation on that finding instead of adding a separate Architecture finding." Compliance varies by model, so this alone is not enough.
2. **Deterministic merge (recommended, together with 1):** after validation and anchoring, fold pairs where:
   - both findings are in the same file with overlapping line ranges;
   - exactly one is Architecture *with* citations; and
   - the other shares a root cause.

   The merged finding keeps the higher severity, the union of citations, the non-Architecture title and body with the ADR named, and a span that covers both ranges. "Shares a root cause" needs a conservative test, for example the identical span plus a match on title keywords, so that two genuinely different problems on one line stay separate: `eval` (Critical) next to a `no-unsafe-return` (Minor) must not merge.
3. **Validation-time rejection:** treat a same-span Security/Architecture pair as invalid and send it back through the repair loop. This is deterministic, but costs an extra model call and couples the schema boundary to a style rule.

## Open question
Which category should a merged finding keep? Keeping Security plus citations loses VZ-002's Architecture signal. Keeping Architecture understates severity labeling. A likely answer: keep the problem's own category, and change VZ-002 to "any finding citing a knowledge document is an architecture violation". That needs a coordinated garmr policy-pack change.

## Acceptance criteria
- [x] Decide the option(s) and the category rule, recorded as an ADR or an amendment to ADR-0007
- [x] Unit test with the four observed pair shapes: each folds into one finding with max severity and the union of citations
- [x] Unit test showing that two distinct problems on the same span (e.g. `eval` + `no-unsafe-return`) do **not** merge
- [x] Merge stays deterministic: the same inputs in any chunk order give the same output
- [x] garmr policy pack and `garmr test` suites updated if VZ-002's trigger changes (not needed: the merged finding stays Architecture)
- [x] vizzini-lab live rerun of `ts-seeded`: Opus reports one finding per problem (11, with 10/10 recall still met), and Sonnet's output is unchanged

## Resolution (2026-11-11)
Implemented as [ADR-0012](/decisions/adr-0012-fold-same-span-duplicate-findings-that-cite-the-same-knowledge-document--01a1cb3ca045.md), with the category rule decided as **Architecture**, so garmr VZ-002 is unchanged. Two parts:
1. **Prompt rule:** the knowledge section asks for one finding per problem.
2. **Deterministic fold** in `Engine.Review`: same file, overlapping lines, exactly one Architecture, a shared citation. It keeps the higher severity, the specific title, both bodies, the union of the spans, and all citations. The count goes in the new `merged_findings` (closed `#ReviewReport`).

**Tests:**
- `TestFoldDuplicates_*`: the four live pairs, seven look-alikes that must not fold, all 24 input orderings, and that the result still validates.
- `TestReview_FoldsSameSpanDuplicates`: end to end through the mock LLM, including the prompt rule and the report field.

**Live rerun** (vizzini-lab run 20261111T153406Z, `ts-seeded`, no forge leg):

| | before (20261111T150647Z) | after |
|---|---|---|
| Opus 5.5 findings | 15 (4 duplicate pairs) | **11**, 10/10 recall |
| Sonnet 5.5 findings | 11 | 11, 10/10 recall |
| Hard checks | 46/46 | 36/36 |

`merged_findings` was 0 for both models: the prompt rule alone stopped the duplicates this run, so the fold didn't need to act. It remains the deterministic backstop for when a model ignores the rule. Model output varies between runs, so this is one observation, not a guarantee.
