---
id: "01a1cc2b-d91a-7559-b504-60b52d7e6db7"
title: "ADR-0017: Thread memory matches by fingerprint or mapped anchor, and resolves only when the code changed"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "forge"
created: "2026-11-11T13:53:08-06:00"
timestamp: "2026-11-11T13:53:08-06:00"
description: "vizzini-bot recognizes its earlier threads by a hidden marker, treats a finding as already posted when its fingerprint or its file, category and git-mapped lines match, and resolves a thread only when its finding is gone and the code under it changed."
---

# ADR-0017: Thread memory matches by fingerprint or mapped anchor, and resolves only when the code changed


**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 2
**Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md) (thread memory lives in the forge)

## Context
A bot that reviews every push must not post the same finding twice, and should close what it raised once the code is fixed. Two facts make the obvious approach wrong:
- **Titles drift.** A live model rewords a finding between runs, for example "SQL injection" one time and "Query built by string concatenation" the next. A fingerprint built from the title alone would post the same finding again.
- **Findings come and go.** A live model is not deterministic: a finding can go unreported in one run while its code is unchanged. Resolving a thread whenever its finding is absent would close real problems by chance.

## Decision
- **The marker:** every posted comment ends with a hidden marker,
  `<!-- vizzini:fp=v1:… run=… head=<sha> lines=<s>-<e> cat=<category> -->`. It records the fingerprint, the run, and where the comment was anchored, at which head. The bot keeps no store of its own.
- **Reading the threads back:** a run lists the PR's threads (`mrman review threads`) and maps every earlier anchor to the current head with `git diff -U0 <then> <now> -- <file>`. Each anchor ends up in one of three states:
  - *same*, shifted by the hunks above it;
  - *changed*, when a hunk touches its lines or the file is gone;
  - *unknown*, when the old head is not in the checkout, as in a shallow clone.
- **Already posted:** a finding counts as already posted when a thread has the same fingerprint, or the same file and category on overlapping lines after mapping. Exception: a thread that is resolved and whose code changed was fixed, so the same finding again is a regression and is posted anew. A thread a person resolved while its code stood still stays resolved, which makes dismissing a finding stick.
- **Resolved:** an open thread is resolved, with a reply, only when no finding matches it **and** its code changed. It goes through `mrman review resolve` under the grant policy, and mrman only resolves threads the token's own identity started.
- **Kept open:** an open thread whose finding went unreported while its code is unchanged, or could not be mapped, stays open and is reported as `kept_open`.

## Consequences
- **Duplicates by construction:** a finding on lines that already carry a thread of the same category is never posted again. Two genuinely different findings of the same category on the same lines share one thread.
- **Conservative resolution:** a fix that changes code elsewhere, such as a caller, leaves the thread open for a person. Resolution needs a full-history checkout: CI must fetch the earlier heads (`fetch-depth: 0`), or anchors stay unknown and nothing is resolved.
- **Verified live:** vizzini-lab ADR-0005, 21/21 on an ADO PR with Sonnet 5.5. The rerun had no duplicates. The pushed fix resolved exactly the MD5 thread and kept one variance-only thread open.
