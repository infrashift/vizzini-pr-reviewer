---
id: "01a1cf97-e0c1-7af8-a933-b282b8f38858"
title: "ISSUE-029: /vizzini reconsider: the bot re-evaluates a disputed finding"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "automation"
  - "commands"
  - "agents"
  - "open"
created: "2026-11-12T05:49:59-06:00"
timestamp: "2026-11-12T05:49:59-06:00"
description: "A structured disagreement: a developer argues in the bot's thread and writes /vizzini reconsider; the bot re-evaluates the finding against the code and the argument, then withdraws it (resolving its own thread) or explains why it stands, leaving the dispute to a person."
---

# ISSUE-029: /vizzini reconsider: the bot re-evaluates a disputed finding

**Status:** open (verified live on Azure DevOps) · **Decision:** [ADR-0023](/decisions/adr-0023-disputed-findings-the-bot-may-withdraw-its-own-finding-anything-else-goes-to-a-person--01a1cf9d793e.md) · **Raised:** 2026-11-12 · **Related:** [ADR-0019](/decisions/adr-0019-in-thread-vizzini-commands-are-answered-once-by-the-next-bot-run--01a1cc771df6.md) (commands), [ISSUE-027](/issues/issue-027-mrman-findings-view-for-agents-review-threads-findings--01a1cf97e098.md), [ISSUE-028](/issues/issue-028-mrman-reply-and-resolve-on-github-and-gitlab--01a1cf97e0b0.md), [ISSUE-026](/issues/issue-026-webhook-receiver-answers-vizzini-commands-immediately-experimental--01a1cf8f51ab.md)

## Context: the developer-agent workflow
These three issues come from one workflow: Vizzini reviews, and a developer, a person or an agent, either fixes or argues back. Any disagreement still goes to a person to decide. What already works:
- **Read:** `mrman review threads` lists the bot's threads.
- **Fix:** a pushed fix is checked by the bot itself, which resolves its own thread ([ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 2).
- **Defend:** a reply does not make the bot back down.
- **Settle:** a person resolves the thread as *won't fix* and the bot suggests a suppression rule (slice 5).

Guardrails that exist today:
- mrman's `review resolve` closes **only threads the token's own identity started**. A developer agent with its own account therefore cannot resolve vizzini-bot's threads.
- Unattended posting goes only through a base-branch garmr policy ([ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md)), and mrman's interactive grant stays human-only.

The vizzini-uat suite TS-WF tests the whole workflow.

## Goal
A structured disagreement. The developer argues in the bot's thread and writes `/vizzini reconsider`. The bot re-evaluates **that finding** against the current code and the thread's conversation. The model's answer is checked against the CUE schema.

## Acceptance criteria
- [x] The command `/vizzini reconsider [argument]` is valid only in a thread that carries a Vizzini finding marker. Anywhere else it gets one usage reply.
- [x] The model returns `{"verdict": "withdrawn" | "stands", "explanation": "…"}`, validated against `schema/review.cue`.
- [x] **withdrawn:** the bot resolves its own thread (`mrman review resolve` with the explanation as reply), and the finding is not posted again on later runs at the same code. It is matched as already raised and, being resolved, left alone.
- [x] **stands:** the bot replies with the explanation and leaves the thread **open**, so a person decides.
- [x] Answered once (the `reply-to` marker). The event is `resolve` or `reply` under the base-branch policy.
- [x] Attributed like other commands (mode `command`, kind `reconsider`).
- [x] The webhook receiver needs no change: a reconsider is a command.
- [x] vizzini-uat TS-WF covers both verdicts with a deterministic stub.

## Progress (2026-11-12)
- **Vizzini `0187f14`:** `/reconsider` (stubllm: `[stub:withdraw]`).
- **Live:** vizzini-uat TS-WF, run 20261112T121641Z, passed 26/26 on ADO PR #47.
  - The stands verdict left the thread open.
  - The withdrawn verdict resolved the bot's own thread, and a later full review did not post the finding again.
- **Still to do:** a live-model run with a real argument.
