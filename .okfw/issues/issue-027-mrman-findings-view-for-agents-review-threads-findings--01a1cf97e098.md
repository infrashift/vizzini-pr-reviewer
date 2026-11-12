---
id: "01a1cf97-e098-7df6-abf1-d19e782553b9"
title: "ISSUE-027: mrman findings view for agents (review threads --findings)"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "mrman"
  - "automation"
  - "agents"
  - "open"
created: "2026-11-12T05:49:59-06:00"
timestamp: "2026-11-12T05:49:59-06:00"
description: "A structured view of vizzini-bot's threads for developer agents: mrman review threads --findings decodes each thread's hidden marker into fingerprint, file, lines, category, suggestion and status, so agents never parse comment text."
---

# ISSUE-027: mrman findings view for agents (review threads --findings)

**Status:** open · **Raised:** 2026-11-12 · **Related:** [ISSUE-028](/issues/issue-028-mrman-reply-and-resolve-on-github-and-gitlab--01a1cf97e0b0.md), [ISSUE-029](/issues/issue-029-vizzini-reconsider-the-bot-re-evaluates-a-disputed-finding--01a1cf97e0c1.md)

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
A developer agent needs the findings, not prose. Today it would have to regex the hidden `<!-- vizzini:fp=… -->` marker out of comment bodies, and that format belongs to vizzini-bot.

## Acceptance criteria
- [x] Vizzini owns the marker format in a public package, `pkg/marker`: parse and format, versioned, tested. vizzini-bot uses it instead of its private copy.
- [ ] `mrman review threads --findings` prints one JSON object for each thread that carries a Vizzini marker: thread id, status (open or resolved), file, start and end lines, category, fingerprint, run, head, title, body, the suggested fix if any, and the replies after the finding.
- [ ] Threads without a marker are left out, and malformed markers are reported, not fatal.
- [ ] Unit tests in both repositories. The vizzini-uat workflow suite reads findings only through this view.
