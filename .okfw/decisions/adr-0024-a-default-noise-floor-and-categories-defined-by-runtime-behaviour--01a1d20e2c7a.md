---
id: "01a1d20e-2c7a-7f09-8f58-1c3567f603b1"
title: "ADR-0024: A default noise floor, and categories defined by runtime behaviour"
type: "ADR"
tags:
  - "adr"
  - "review"
  - "quality"
created: "2026-11-12T17:18:26-06:00"
timestamp: "2026-11-12T17:18:26-06:00"
description: "Trivial and Minor Performance findings move to the report's filtered list by default (never posted, still gated on), and the review prompt defines categories by runtime behaviour; chosen and validated on the ISSUE-022 benchmark."
---

# ADR-0024: A default noise floor, and categories defined by runtime behaviour

**Status:** Accepted
**Date:** 2026-11-12
**Related:** [ISSUE-032](/issues/issue-032-precision-about-half-of-vizzinis-findings-are-not-real-problems--01a1d1cb7579.md), [ADR-0003](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md) (policy lives in the CUE schema), [ADR-0020](/decisions/adr-0020-suppression-rules-are-reviewed-repository-files-read-from-the-base-branch-and-applied-deterministically--01a1cca0ea51.md) (suppression rules)

## Context
The benchmark against PR-Agent (ISSUE-022) found Vizzini's main weakness. On real pull requests, judged blind, only about half its findings were real problems. The noise had a clear signature:
- **Trivial findings:** 6% were real problems.
- **Minor Performance and Maintainability findings:** about a fifth were real problems.
- **Major findings:** 81% were real problems.

## Decision
- **A default noise floor.** `#NoiseFloor` in `schema/review.cue` holds rules: a severity, optionally narrowed to categories. The default filters Trivial findings and Minor Performance findings.
- **Filtered, not dropped.** A matching finding moves from the report's `findings` to a new `filtered` list. Nothing is silently dropped.
- **Shown, never decisive.** The floor decides what is shown and posted, never what blocks: `--fail-on` and auto-approval still see filtered findings.
- **The bot.** `vizzini-bot` does not post filtered findings. It matches them against its threads like suppressed ones, so it never resolves a thread as fixed only because the floor now hides that finding.
- **Turning it off.** `Config.NoiseFloor` replaces the rules, and `{"rules": []}` turns the floor off. `ReviewOptions.AllFindings` and `pr-review --all-findings` turn it off for one review.
- **A category rule in the review prompt.** Each category is defined by what happens when the code runs. A behaviour bug is Logic, never Maintainability, even when its cause looks structural.

## Why not suppression rules
Suppression rules are a repository's own reviewed decisions. The schema requires each one to name a fingerprint, paths or a title, so that no rule can silence a whole class of findings. A built-in, severity-wide default is a different thing, so it gets its own small definition.

## Why this floor, and not a wider one
- **Measured on fresh reviews.** Both changes were measured on new benchmark runs, with every new finding judged blind.
- **The wider floor we tried.** It also filtered Minor Maintainability findings. It gave the most precision, but in one run it hid 2 of 29 real defects: bugs the model had filed under Maintainability. The owner chose the narrower floor, and the category rule targets that failure.
- **Result** (Vizzini `38d9959` against `b002fad`, on 31 real cases):
  - strict precision rose from 51% to 69% (Sonnet 5.5), +18 points [9, 28], and from 50% to 68% (Opus 5.5), +19 points [11, 26];
  - recall rose 3 points with each model;
  - the floor alone added 11 to 13 points of precision with no defect lost.

## Consequences
- **Fewer findings.** Reviews show about 20–30% fewer findings. Teams that want every nit pass `--all-findings`.
- **What it depends on.** The floor relies on the model's severity and category. A real bug that the model rates Trivial or Minor Performance is hidden from the output, though it stays in the JSON's `filtered` list.
- **Changing the defaults.** Any change to the floor's defaults should be measured on the benchmark (`uat/bench.sh --tools vizzini`, `uat/bench_compare.py`) before it ships.
