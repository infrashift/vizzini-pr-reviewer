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
  - "open"
created: "2026-11-11T11:24:31-06:00"
timestamp: "2026-11-11T11:24:31-06:00"
description: "Turn 'better than open-source alternatives' into numbers: run PR-Agent (and Kodus if practical) on the same fixtures and on hand-labelled real PRs through the same LiteLLM proxy, and report precision as well as recall."
---

# ISSUE-022: Head-to-head review benchmark against PR-Agent and Kodus

**Status:** open · **Raised:** 2026-11-11 · **Harness:** vizzini-uat ([ISSUE-016](/issues/issue-016-cross-tool-uat-on-c-with-live-claude-sonnet-5-5-and-opus-5-5--01a1caee2585.md))

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
- [ ] PR-Agent runs in vizzini-uat against the same proxy, with output normalized to (file, lines, severity, text)
- [ ] Labelled real-PR set with provenance (repo, commits, license) checked in as pins, not code
- [ ] `score.py` reports precision and recall per tool, model and fixture, plus a comparison table
- [ ] First published comparison recorded here, with methodology and caveats
