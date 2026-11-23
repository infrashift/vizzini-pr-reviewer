---
x-okfw-id: "01a206f0-275f-7187-b9c8-7d2c7c28d52c"
title: "ADR-0028: A review that posts nothing records its head in one edited status comment"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "cost"
x-okfw-created: "2026-11-22T23:45:31-06:00"
x-okfw-timestamp: "2026-11-22T23:45:31-06:00"
description: "A quiet review records the head it reviewed in one pull-request-level status comment that the bot posts once and then edits through mrman review edit; commands-only answers record no head."
---

# ADR-0028: A review that posts nothing records its head in one edited status comment

**Status:** Accepted
**Date:** 2026-11-22
**Source:** vizzini-lab ISSUE-002 (runs that post nothing do not record their head)
**Related:** [ADR-0018](/decisions/adr-0018-incremental-review-by-file-since-the-last-reviewed-head-learned-from-thread-markers--01a1cc54bd32.md) (incremental review), [ADR-0022](/decisions/adr-0022-commands-are-answered-from-a-forge-webhook-by-a-commands-only-bot-run--01a1cf8f51be.md) (commands-only runs), [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md) (posting under a garmr policy)

## Context
The bot learns the last reviewed head from the markers in its own comments ([ADR-0018](/decisions/adr-0018-incremental-review-by-file-since-the-last-reviewed-head-learned-from-thread-markers--01a1cc54bd32.md)). A review that posts nothing (no finding, no resolution, no answer, no suggestion) left no marker. The next run then reviewed again from an older head: more model calls and more chances to vary on code that was already reviewed. ADR-0018 named the fix, a summary marker, but mrman could not edit a comment.

A second gap had the opposite effect. A `--commands-only` run ([ADR-0022](/decisions/adr-0022-commands-are-answered-from-a-forge-webhook-by-a-commands-only-bot-run--01a1cf8f51be.md)) reviews nothing, but its answers recorded the current head. If the CI review of that head had failed, the next review started after it, and that push's files were never reviewed.

## Decision
- **One status comment per pull request.** A review run (mode `full` or `incremental`) that posted nothing which records its head writes the head to a pull-request-level comment marked `<!-- vizzini:status -->`.
  - The first such run posts it (`mrman review add` without a file, then `review submit`, event `comment`).
  - Later runs edit it (`mrman review edit --summary ID`, event `edit`). mrman v0.1.11 adds the command; mrman edits only comments the token's own identity wrote.
- **What it holds:** one `<!-- vizzini:reviewed head=… run=… -->` line per head, the last 20, oldest first. The bot reads them with the same pattern as every other head marker. A head reviewed again moves to the end.
- **When it writes nothing:**
  - the run posted a finding, a resolution, an answer or a suggestion (each records the head itself);
  - the head is already recorded;
  - the mode is `unchanged` or `commands`;
  - mrman lists no pull-request-level comments (older than v0.1.11). Without that list the bot cannot find its status comment, and posting a new one on each quiet run would flood the pull request.
- **Failure is not fatal.** A refused or failed write sets `status: "failed"` and is logged. A policy that lacks `edit` is named in the log. The next run reviews more than needed, never less.
- **On Azure DevOps the status thread is resolved once it is posted** (`mrman review resolve`, event `resolve`, no reply). A pull-request-level comment there is a thread, created active. A branch policy that requires every comment to be resolved would otherwise block the pull request on a note that asks nothing. A later edit leaves the thread's status alone. GitHub review bodies and GitLab general notes have no such state.
- **Commands-only answers record no head.** Their `reply-to` marker omits `head=`, which the marker format already allows.

## Consequences
- A pull request with quiet pushes gets one extra comment, edited in place, instead of repeated reviews of the same files.
- Policies need `edit` in `_allowedEvents`. The policy pack in `deploy/ci/grant-policy` and mrman's default pack add it. A policy without it keeps the old behaviour and logs why.
- The templates need an mrman release with `review edit` (v0.1.11) before the status comment works in CI.
- **Trust:** like the other head markers, the status comment is found by its marker, not by its author. A person who writes a status marker can make the bot skip files, as they already can with a finding marker. Only the bot's own comment can be edited, so a forged status comment makes the edit fail (`status: "failed"`); it does not change what the bot writes.
