---
id: "01a1cba3-ca8f-722d-a3bb-2d8f4ada063b"
title: "ISSUE-022: Head-to-head review benchmark against PR-Agent and Kodus"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "uat"
  - "benchmark"
  - "quality"
  - "closed"
created: "2026-11-11T11:24:31-06:00"
timestamp: "2026-11-11T11:24:31-06:00"
description: "Turn 'better than open-source alternatives' into numbers: run PR-Agent (and Kodus if practical) on the same fixtures and on hand-labelled real PRs through the same LiteLLM proxy, and report precision as well as recall."
---

# ISSUE-022: Head-to-head review benchmark against PR-Agent and Kodus

**Status:** closed · **Raised:** 2026-11-11 · **Harness:** vizzini-lab ([ISSUE-016](/issues/issue-016-cross-tool-uat-on-c-with-live-claude-sonnet-5-5-and-opus-5-5--01a1caee2585.md))

## Why
The live UATs measure **recall** on planted, fairly textbook defects. Both models score 100%. Nothing measures **precision** (false positives) on real PRs, and nothing compares Vizzini with the open-source tools people would otherwise deploy. Third-party write-ups note there are no independent accuracy benchmarks for these tools, so a published, reproducible comparison is itself valuable.

## Scope
- **Competitors:**
  - PR-Agent (community-maintained, formerly Qodo's open-source reviewer): `/review` and `/improve` from its CLI or Docker image, pinned by digest, pointed at the same LiteLLM proxy and models.
  - Kodus, if self-hosting it is practical in podman.
- **Inputs:**
  - The seeded C# and TypeScript fixtures (ground truth exists).
  - A **labelled real-PR set**: 20–30 historical PRs from permissive OSS repos whose later fix commits identify a real bug the PR introduced ("bug-introducing commit" + "fixing commit" pairs). Each is labelled with the defect lines.
  - A few clean PRs, to measure false positives.
  - **Context-dependent defects** (from ISSUE-023's live run): seeded changes whose bug is visible only outside the diff, run with and without `--repo` as an A/B pair. Examples: a signature change that breaks an unchanged caller, a dereference of a callee that can return null, or a duplicate of validation the callee already does.
- **Metrics per tool and model:**
  - recall and precision (a finding counts as true if it overlaps a labelled defect, or if a human judges it valid);
  - findings per PR and decoy hits;
  - latency, tokens and cost (from LiteLLM spend logs, see ISSUE-025).
- **Judging:** an unmatched finding is labelled by hand once, then reused across runs. Optional LLM-as-judge runs only as a second opinion.

## Acceptance criteria
- [x] PR-Agent runs in vizzini-lab against the same proxy, with output normalized to (file, lines, severity, text)
- [x] Labelled real-PR set with provenance (repo, commits, license) checked in as pins, not code
- [x] `score.py` reports precision and recall per tool, model and fixture, plus a comparison table
- [x] First published comparison recorded here, with methodology and caveats

## Resolution (2026-11-12)
The benchmark is in vizzini-lab as `make bench` (`uat/bench.sh`, `uat/bench.py`; method in lab ADR-0013). The comparison is published on the docs site: [Benchmark: Vizzini vs PR-Agent](https://infrashift.github.io/vizzini-pr-reviewer/docs/project/benchmark/).

**Setup:**
- **Tools:** Vizzini `b002fad` against PR-Agent 0.49.0 (pinned image), with Claude Sonnet 5.5 and Opus 5.5 through one LiteLLM proxy.
- **Cases:** 34 in all.
  - 26 real bug-introducing changes from 20 projects, each named by a later fix, 15 of them after the models' cutoff;
  - 5 clean changes;
  - 3 seeded fixtures, among them a new context-dependent Go fixture.
- **Rounds:**
  1. equal inputs;
  2. plus lint for Vizzini;
  3. plus a blind-written OKF wiki per project, also given to PR-Agent as `extra_instructions`.
- **Judging:** all 1,350 findings were judged blind against a published rubric. A second judge agreed on 91% of a 150-finding sample (κ 0.90). The owner reviewed the disputed calls.

**What it showed, on real pull requests:**
- **Recall:** about the same; no difference holds. Vizzini with Sonnet 5.5 found 79% and PR-Agent 69%; with Opus 5.5, Vizzini found 69% and PR-Agent 72%.
- **Precision:** PR-Agent is more precise, by 26 points with Opus (this holds). Follow-up: [ISSUE-032](/issues/issue-032-precision-about-half-of-vizzinis-findings-are-not-real-problems--01a1d1cb7579.md).
- **Time and cost:** Vizzini takes about a third of the time and half the cost.
- **Lint and knowledge:** neither changed Vizzini's results measurably.
- **Repository context ([ISSUE-023](/issues/issue-023-repository-context-for-reviews-definitions-and-callers-outside-the-diff--01a1cba3caa3.md)):** found all 3 context-dependent defects, against 2 without it, and added 6 points of precision with Opus. Its effect on recall over the real pull requests did not reach significance.
- **Seeded fixtures:** Vizzini's 100% there mostly reflects PR-Agent's 3-issue cap meeting fixtures with up to 10 defects in one change.

**Kodus:** deferred. Self-hosting it means several services, and the owner chose PR-Agent only for this round.

