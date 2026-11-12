---
id: "01a1cc54-bd32-726f-910d-1744be43ff5d"
title: "ADR-0018: Incremental review by file since the last reviewed head, learned from thread markers"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "cost"
created: "2026-11-11T14:37:47-06:00"
timestamp: "2026-11-11T14:37:47-06:00"
description: "vizzini-bot learns the last reviewed head from its own thread markers and reviews only the pull request's files changed since then (the base...head diff, limited to those files); it carries threads on other files and falls back to a full review when no earlier head is usable."
---

# ADR-0018: Incremental review by file since the last reviewed head, learned from thread markers


**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 3
**Related:** [ADR-0017](/decisions/adr-0017-thread-memory-matches-by-fingerprint-or-mapped-anchor-and-resolves-only-when-the-code-changed--01a1cc2bd91a.md) (thread memory), [ADR-0015](/decisions/adr-0015-cost-attribution-through-litellm-spend-logs-correlated-by-run-and-pull-request--01a1cbc7d120.md) (cost)

## Context
Before this change, every push re-reviewed the whole pull request. That was costly, re-reviewed code nobody touched, and gave the model more chances to vary. Three questions needed an answer:
- where to remember what was last reviewed, without a store;
- what "the push's changes" means when the branch also merges its base;
- how to treat earlier threads on code this run does not look at.

## Decision
- **The last reviewed head comes from the markers.** Each posted finding's marker already records its head (ADR-0017). The last reviewed head is the newest marker head that is an ancestor of the current head (`git merge-base --is-ancestor`), meaning the fewest commits away.
- **Scope by file, within the PR's own diff.** The review is the pull request's diff (`base...head`) limited to the files `git diff since head` touches.
  - Changes merged in from the base branch are not in `base...head`, so they are never reviewed.
  - The model still sees each file's whole PR diff as context, not just the newest hunk.
- **Unchanged means no review.** When the head has not moved, or the push touches no file the PR changes (for example a merge from the base branch), no model call is made.
- **Carry, don't judge.** Open threads on files outside the scope are carried as they are: not matched, not kept open, not resolved.
- **Fall back to full.** The bot reviews the whole PR when:
  - there is no usable earlier head (a first run, a rebased or force-pushed branch, or a shallow clone);
  - `--full` is passed.

## Consequences
- **Measured on vizzini-lab (ADO PR #22, Sonnet 5.5):**
  - An incremental review of a one-file push cost $0.0081 (1,994 input tokens), against $0.051–$0.059 for a full review.
  - It took 16s instead of 43–50s.
  - An unchanged head cost nothing.
- **Runs that post nothing leave no marker.** The next run then reviews from the last head that did post: more than needed, never less. A summary marker could record every reviewed head, but mrman cannot edit a comment yet.
- **File granularity.** A one-line push to a large file re-reviews that file's whole PR diff. Hunk-level scoping is possible later.
- **CI needs full history** (`fetch-depth: 0`). This is the same requirement as resolution.
