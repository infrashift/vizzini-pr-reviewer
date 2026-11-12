---
id: "01a1d1cb-7579-7d52-854d-079bfeea9e12"
title: "ISSUE-032: Precision: about half of Vizzini's findings are not real problems"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "quality"
  - "benchmark"
  - "open"
created: "2026-11-12T16:05:34-06:00"
timestamp: "2026-11-12T16:05:34-06:00"
description: "The ISSUE-022 benchmark shows Vizzini's strict precision at about 50% on real pull requests, against 63–76% for PR-Agent at the same recall; find and measure a change that cuts the noise."
---

# ISSUE-032: Precision: about half of Vizzini's findings are not real problems

**Status:** open · **Raised:** 2026-11-12 · **Found by:** [ISSUE-022](/issues/issue-022-head-to-head-review-benchmark-against-pr-agent-and-kodus--01a1cba3ca8f.md)'s benchmark ([page](https://infrashift.github.io/vizzini-pr-reviewer/docs/project/benchmark/))

## Problem
On 26 real bug-introducing pull requests, judged blind, about half of Vizzini's findings were real problems (strict precision 50–51%). For PR-Agent the figure was 63% with Sonnet 5.5 and 76% with Opus 5.5. With Opus the gap holds statistically: 26 points [15, 36]. Recall is about the same for both tools, so the noise does not buy Vizzini more found bugs. Vizzini also raised the most findings on clean changes (12 with Opus, 3 of them invalid).

Most of the findings that were not real problems were nits: style, unreachable defensive checks, and "consider a test" with no specific risk. Invalid findings were mostly speculation about code the model could not see.

## Ideas to evaluate (each measured on the same benchmark, `make bench`)
- Tell the review prompt to drop nits, or to report them separately, below a severity threshold.
- A second verification pass: ask the model to check each finding against the code and the context, and drop the ones it cannot support (similar to `/reconsider`, ADR-0023).
- A cap, or a "confidence" field enforced by the CUE schema, with a default filter.

## Acceptance criteria
- [ ] One change, chosen by the benchmark, that raises strict precision on the real pull requests without losing recall beyond its interval
- [ ] The benchmark page updated with the new numbers, the old ones kept for comparison
