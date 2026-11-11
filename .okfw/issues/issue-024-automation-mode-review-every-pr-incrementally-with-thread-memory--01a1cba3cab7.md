---
id: "01a1cba3-cab7-7980-8a95-e7ac7ef7008f"
title: "ISSUE-024: Automation mode: review every PR, incrementally, with thread memory"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "automation"
  - "forge"
  - "mrman"
  - "open"
created: "2026-11-11T11:24:31-06:00"
timestamp: "2026-11-11T11:24:31-06:00"
description: "Review PRs automatically on open and push (CI action or webhook), re-review only new changes, track Vizzini's own threads (skip duplicates, resolve fixed ones), and learn from dismissed findings — keeping mrman's human grant for anything beyond comments."
---

# ISSUE-024: Automation mode: review every PR, incrementally, with thread memory

**Status:** open · **Raised:** 2026-11-11 · **Touches:** [ISSUE-009](/issues/issue-009-interactive-pr-slash-commands--01a1c9b61586.md) (slash commands), mrman's agent-submit grant

## Problem
Vizzini is a library and CLI. Nothing reviews a PR when it is opened or updated, slash commands work only from the CLI (not in PR threads), and nothing remembers what was already said. Open-source alternatives install as a bot.

## Scope
- **Triggers:** a container image / CI step (GitHub Actions, Azure Pipelines, GitLab CI) and an optional small webhook receiver. Both call the same entry point.
- **Incremental review:** compare the PR's previous reviewed head SHA with the new one and review only the inter-diff, while carrying forward still-valid findings.
- **Thread memory:** a fingerprint per finding (file, a line-independent anchor such as normalized code text, category and root cause) stored in the forge thread (hidden marker). Re-runs skip already-posted findings, and a thread is resolved when the code no longer shows the problem.
- **In-thread commands:** `/vizzini explain|refactor|docstrings` in a PR comment runs ISSUE-009's commands and replies in the thread.
- **Feedback:** a dismissed or "won't fix" finding becomes a per-repository suppression rule (fingerprint plus scope), which is fed back as a knowledge-style document.
- **Governance:** the bot posts only comment or draft events by default. Approve and request-changes need an explicit, auditable policy (garmr), mirroring mrman's human grant.

## Acceptance criteria
- [ ] Design ADR: where state lives (forge thread markers vs a store), forge abstraction (reuse mrman's drivers?), and the posting policy
- [ ] CI entry point posts a review on a test PR (ADO first, since vizzini-uat already has it)
- [ ] Incremental re-review on push, with no duplicate threads, verified in vizzini-uat
- [ ] Thread resolution when fixed, and suppression from dismissal
