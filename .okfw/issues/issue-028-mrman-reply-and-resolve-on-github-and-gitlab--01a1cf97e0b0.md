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
  - "closed"
created: "2026-11-12T05:49:59-06:00"
timestamp: "2026-11-12T05:49:59-06:00"
description: "Implement mrman's ThreadReplier and ThreadResolver for GitHub (GraphQL reviewThreads) and GitLab (discussions), with the same own-thread-only resolve rule as Azure DevOps, so vizzini-bot resolves fixed findings and answers commands on every forge."
---

# ISSUE-028: mrman reply and resolve on GitHub and GitLab

**Status:** closed (released in mrman v0.1.6; verified live on GitHub, GitLab and Azure DevOps) · **Raised:** 2026-11-12 · **Related:** [ISSUE-027](/issues/issue-027-mrman-findings-view-for-agents-review-threads-findings--01a1cf97e098.md), [ISSUE-029](/issues/issue-029-vizzini-reconsider-the-bot-re-evaluates-a-disputed-finding--01a1cf97e0c1.md); vizzini-lab ISSUE-002

## Context: the developer-agent workflow
These three issues come from one workflow: Vizzini reviews, and a developer, a person or an agent, either fixes or argues back. Any disagreement still goes to a person to decide. What already works:
- **Read:** `mrman review threads` lists the bot's threads.
- **Fix:** a pushed fix is checked by the bot itself, which resolves its own thread ([ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 2).
- **Defend:** a reply does not make the bot back down.
- **Settle:** a person resolves the thread as *won't fix* and the bot suggests a suppression rule (slice 5).

Guardrails that exist today:
- mrman's `review resolve` closes **only threads the token's own identity started**. A developer agent with its own account therefore cannot resolve vizzini-bot's threads.
- Unattended posting goes only through a base-branch garmr policy ([ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md)), and mrman's interactive grant stays human-only.

The vizzini-lab suite TS-WF tests the whole workflow.

## Goal
Only the Azure DevOps driver implements mrman's optional `ThreadReplier` and `ThreadResolver`. On GitHub and GitLab, vizzini-bot cannot resolve fixed findings or answer `/vizzini` commands, and a developer cannot defend a finding through mrman.

## Acceptance criteria
- [x] **GitHub:**
  - reply through GraphQL `addPullRequestReviewThreadReply`;
  - resolve through `resolveReviewThread`, only when the thread's first comment was written by the viewer (`ErrorForbidden` otherwise);
  - an already-resolved thread is left untouched.
- [x] **GitLab:**
  - reply with a note in the discussion;
  - resolve with `resolved=true` on the discussion, with the same own-thread rule and idempotence.
- [x] `mrman review reply` and `review resolve` work on both forges, under the human grant and under `--grant-policy`.
- [x] Tests with recorded responses for each driver; `make check`, including the 85% coverage gate.
- [x] Live: vizzini-lab's GitHub and GitLab CI template tests answer a `/vizzini` command and resolve a fixed finding. Both forges do both: GitHub (resolving with the `contents: write` opt-in) and GitLab (vizzini-lab run 20261112T134151Z, 16/16: the bot user answered the command and resolved its own discussion on a file the push removed).

## Progress (2026-11-12)
- **mrman, branch `feature/github-gitlab-reply-resolve`:**
  - GitHub: reply through GraphQL `addPullRequestReviewThreadReply`, resolve through `resolveReviewThread`, with the own-thread rule. A thread from another PR is refused, because GitHub thread ids are global.
  - GitLab: a discussion note, and a discussion PUT with `resolved`.
  - `make check` passes (87.5% coverage).
- **Not yet verified live:**
  - whether a GitHub reply lands in a pending review;
  - the `[bot]` login suffix for GitHub App tokens;
  - GraphQL permission messages as GitHub really sends them.
- **What closes it:** a mrman release pinned in the CI templates, then vizzini-lab's GitHub and GitLab tests.

## Released and verified (2026-11-12)
- **Release:** mrman v0.1.6 (PRs #16 and #17), pinned in every CI template (Vizzini `9e706bc`).
- **Live results** (vizzini-lab `results/2026-11-12-live-mrman-v016.md`):
  - **GitHub, 15/15:** a `/vizzini` command was answered in its thread as `github-actions[bot]`, and the bot resolved its own thread.
  - **GitLab, 15/15:** the command was answered by the bot user.
  - **Azure Pipelines, 13/13.**
- **GitHub's token permission:** `resolveReviewThread` with `GITHUB_TOKEN` needs `contents: write` (refused without it: "Resource not accessible by integration"). That permission also lets the token push, so the template keeps `contents: read` and documents the opt-in.
- **Found and fixed:** the GitHub test exposed a bot bug. Threads on files a push took out of the PR were carried, not resolved. Fixed in Vizzini `b7f55f9`.
- **Closed (2026-11-12):** the GitLab test now resolves a discussion live (16/16), and mrman v0.1.7 names `contents: write` when GitHub refuses a resolve (PR #18).
