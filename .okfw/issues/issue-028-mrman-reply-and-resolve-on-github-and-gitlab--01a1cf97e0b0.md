---
id: "01a1cf97-e0b0-7237-a44e-a476e6f1f3e8"
title: "ISSUE-028: mrman reply and resolve on GitHub and GitLab"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "mrman"
  - "github"
  - "gitlab"
  - "automation"
  - "open"
created: "2026-11-12T05:49:59-06:00"
timestamp: "2026-11-12T05:49:59-06:00"
description: "Implement mrman's ThreadReplier and ThreadResolver for GitHub (GraphQL reviewThreads) and GitLab (discussions), with the same own-thread-only resolve rule as Azure DevOps, so vizzini-bot resolves fixed findings and answers commands on every forge."
---

# ISSUE-028: mrman reply and resolve on GitHub and GitLab

**Status:** open · **Raised:** 2026-11-12 · **Related:** [ISSUE-027](/issues/issue-027-mrman-findings-view-for-agents-review-threads-findings--01a1cf97e098.md), [ISSUE-029](/issues/issue-029-vizzini-reconsider-the-bot-re-evaluates-a-disputed-finding--01a1cf97e0c1.md); vizzini-uat ISSUE-002

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
Only the Azure DevOps driver implements mrman's optional `ThreadReplier` and `ThreadResolver`. On GitHub and GitLab, vizzini-bot cannot resolve fixed findings or answer `/vizzini` commands, and a developer cannot defend a finding through mrman.

## Acceptance criteria
- [ ] **GitHub:**
  - reply through GraphQL `addPullRequestReviewThreadReply`;
  - resolve through `resolveReviewThread`, only when the thread's first comment was written by the viewer (`ErrorForbidden` otherwise);
  - an already-resolved thread is left untouched.
- [ ] **GitLab:**
  - reply with a note in the discussion;
  - resolve with `resolved=true` on the discussion, with the same own-thread rule and idempotence.
- [ ] `mrman review reply` and `review resolve` work on both forges, under the human grant and under `--grant-policy`.
- [ ] Tests with recorded responses for each driver; `make check`, including the 85% coverage gate.
- [ ] Live: vizzini-uat's GitHub and GitLab CI template tests answer a `/vizzini` command and resolve a fixed finding.
