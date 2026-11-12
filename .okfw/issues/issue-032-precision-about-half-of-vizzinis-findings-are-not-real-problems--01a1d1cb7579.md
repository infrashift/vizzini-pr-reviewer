---
id: "01a1d1cb-7579-7d52-854d-079bfeea9e12"
title: "ISSUE-032: Precision: about half of Vizzini's findings are not real problems"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "quality"
  - "benchmark"
  - "closed"
created: "2026-11-12T16:05:34-06:00"
timestamp: "2026-11-12T16:05:34-06:00"
description: "The ISSUE-022 benchmark shows Vizzini's strict precision at about 50% on real pull requests, against 63–76% for PR-Agent at the same recall; find and measure a change that cuts the noise."
---

# ISSUE-032: Precision: about half of Vizzini's findings are not real problems

**Status:** closed · **Raised:** 2026-11-12 · **Found by:** [ISSUE-022](/issues/issue-022-head-to-head-review-benchmark-against-pr-agent-and-kodus--01a1cba3ca8f.md)'s benchmark ([page](https://infrashift.github.io/vizzini-pr-reviewer/docs/project/benchmark/))

## Problem
On 26 real bug-introducing pull requests, judged blind, about half of Vizzini's findings were real problems (strict precision 50–51%). For PR-Agent the figure was 63% with Sonnet 5.5 and 76% with Opus 5.5. With Opus the gap holds statistically: 26 points [15, 36]. Recall is about the same for both tools, so the noise does not buy Vizzini more found bugs. Vizzini also raised the most findings on clean changes (12 with Opus, 3 of them invalid).

Most of the findings that were not real problems were nits: style, unreachable defensive checks, and "consider a test" with no specific risk. Invalid findings were mostly speculation about code the model could not see.

## Ideas to evaluate (each measured on the same benchmark, `make bench`)
- Tell the review prompt to drop nits, or to report them separately, below a severity threshold.
- A second verification pass: ask the model to check each finding against the code and the context, and drop the ones it cannot support (similar to `/reconsider`, ADR-0023).
- A cap, or a "confidence" field enforced by the CUE schema, with a default filter.

## Acceptance criteria
- [x] One change, chosen by the benchmark, that raises strict precision on the real pull requests without losing recall beyond its interval
- [x] The benchmark page updated with the new numbers, the old ones kept for comparison

## Resolution (2026-11-12)
Two changes, in commit `38d9959` ([ADR-0024](/decisions/adr-0024-a-default-noise-floor-and-categories-defined-by-runtime-behaviour--01a1d20e2c7a.md)):
- **A default noise floor** in `#NoiseFloor`. Trivial and Minor Performance findings move to the report's `filtered` list, are never posted, and still count for the gate and for auto-approval. `--all-findings` turns the floor off.
- **A category rule in the review prompt:** a behaviour bug is Logic, never Maintainability.

**Measured on the benchmark.** These were fresh runs of all 34 cases, with every new finding judged blind. One uniform pass then applied the strict defect rule to every run and both tools.

| Vizzini on 31 real cases | Strict precision | Recall |
|---|---|---|
| Sonnet 5.5, before → after | 51% → 69%, +18 points [9, 28] | 69% → 72% |
| Opus 5.5, before → after | 50% → 68%, +19 points [11, 26] | 59% → 62% |

- **The floor alone:** within the new run, it added 11 to 13 points with no defect lost.
- **Against PR-Agent:** no precision difference holds now. Vizzini is ahead with Sonnet, 69% against 63%; PR-Agent is ahead with Opus, 74% against 68%.

**Tried and not shipped:** a wider floor that also filtered Minor Maintainability findings. It hid 2 of 29 real defects in one run, because the model had filed real bugs under Maintainability.

**Docs:** the benchmark page has a new section, "After ISSUE-032", and the CLI reference covers the noise floor. The lab gained `uat/bench.sh --tools`, `uat/bench_compare.py`, and a `vizzini+filtered` view in `bench.py`.

