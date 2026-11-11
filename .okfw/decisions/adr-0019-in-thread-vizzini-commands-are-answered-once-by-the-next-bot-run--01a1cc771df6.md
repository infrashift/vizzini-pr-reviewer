---
id: "01a1cc77-1df6-7cc6-b412-f47f6db9eaf8"
title: "ADR-0019: In-thread /vizzini commands are answered once, by the next bot run"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "commands"
created: "2026-11-11T15:15:20-06:00"
timestamp: "2026-11-11T15:15:20-06:00"
description: "vizzini-bot answers /vizzini explain|refactor|docstrings written on a line of their own in a PR thread, defaulting the target to the thread's lines, replying through mrman review reply under the grant policy with a reply-to marker that makes each command answered once."
---

# ADR-0019: In-thread /vizzini commands are answered once, by the next bot run


**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 4; [ISSUE-009](/issues/issue-009-interactive-pr-slash-commands--01a1c9b61586.md) (the command engine)
**Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md), [ADR-0018](/decisions/adr-0018-incremental-review-by-file-since-the-last-reviewed-head-learned-from-thread-markers--01a1cc54bd32.md)

## Context
ISSUE-009 gave Vizzini `/explain`, `/refactor` and `/docstrings`, but only from the CLI. People reviewing a pull request ask their questions in its threads, and the bot runs from CI with no webhook. It has to notice the questions on its next run, answer each exactly once, and not let a comment be mistaken for a command.

## Decision
- **Syntax:** a command is `/vizzini <explain|refactor|docstrings> [target] [text]`, on a line of its own in a file-anchored thread. Prose that merely mentions `/vizzini` is not a command, and neither is any comment that carries a vizzini marker (the bot's own).
- **Default target:** without a target, the command is about the thread's own lines (`FILE:START-END`), or its file for `/docstrings`. An explicit `FILE:LINES` overrides it.
- **Context:** the thread's earlier comments go to the model as the conversation, and the PR's `base...head` diff is the scope.
- **Answered once:** the reply goes through `mrman review reply` under the grant policy (event `reply`, a reply to the command comment). It carries `<!-- vizzini:reply-to=<comment id> run=… head=… -->`, and a command with such a reply is never run again.
  - A command that can't be understood, or whose target is outside the diff, gets one usage reply.
  - A model or transport failure gets no reply, so the next run retries.
- **Every run answers commands,** even when no review is needed (an unchanged head). `--max-commands` (default 5) bounds them per run. `--command-authors` limits who may run them; by default anyone who can comment on the PR can.
- **Every bot comment records the head its run reviewed:** finding markers, resolution replies (`<!-- vizzini:reviewed head=… -->`) and command answers. A run that only resolves or answers still advances the last reviewed head (a refinement of ADR-0018).
- **Attribution covers the whole run.** Command requests carry the same run ID, PR key, iteration, requester and agent as the review (ADR-0015).

## Consequences
- **Spend:** a command costs one or two model calls (about $0.014 for a Sonnet 5.5 `/explain` on the UAT fixture). Anyone who can comment can trigger one, so `--command-authors` exists for repositories where that matters, and `--max-commands` caps a run.
- **Untrusted input:** thread text is untrusted, and it reaches the model only as conversation. The answer is schema-validated before it is posted, and it is posted only as a reply.
- **Verified in vizzini-uat** (ADR-0005, PR #26, 37/37):
  - A real `/explain` got an on-point answer about the SQL injection, a malformed command got usage, and nothing was answered twice.
  - The same UAT caught two bugs, both fixed: command requests went unattributed, and resolve-only runs left no head marker.
