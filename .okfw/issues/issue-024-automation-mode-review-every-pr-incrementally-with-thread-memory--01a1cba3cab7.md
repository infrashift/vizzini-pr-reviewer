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
  - "closed"
created: "2026-11-11T11:24:31-06:00"
timestamp: "2026-11-11T11:24:31-06:00"
description: "Review PRs automatically on open and push (CI action or webhook), re-review only new changes, track Vizzini's own threads (skip duplicates, resolve fixed ones), and learn from dismissed findings — keeping mrman's human grant for anything beyond comments."
---

# ISSUE-024: Automation mode: review every PR, incrementally, with thread memory

**Status:** closed (2026-11-11) · **Raised:** 2026-11-11 · **Touches:** [ISSUE-009](/issues/issue-009-interactive-pr-slash-commands--01a1c9b61586.md) (slash commands), mrman's agent-submit grant

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
- [x] Design ADR: where state lives (forge thread markers vs a store), forge abstraction (reuse mrman's drivers?), and the posting policy
- [x] CI entry point posts a review on a test PR (ADO first, since vizzini-lab already has it): slice 1, 2026-11-11
- [x] Incremental re-review on push, with no duplicate threads, verified in vizzini-lab: slices 2-3, 2026-11-11
- [x] Thread resolution when fixed, and suppression from dismissal: slices 2 and 5, 2026-11-11

## Resolution (2026-11-11)
All five slices shipped in a single day. Each was verified live on an Azure DevOps PR by vizzini-lab's `make uat-bot`, which now runs nine legs on one PR (47/47 with Claude Sonnet 5.5).
- **Slice 1:** policy-authorized posting. mrman v0.1.2: `review submit --grant-policy` ([ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md)).
- **Slice 2:** thread memory and resolution. mrman v0.1.3: `review threads` / `review resolve` ([ADR-0017](/decisions/adr-0017-thread-memory-matches-by-fingerprint-or-mapped-anchor-and-resolves-only-when-the-code-changed--01a1cc2bd91a.md)).
- **Slice 3:** incremental review ([ADR-0018](/decisions/adr-0018-incremental-review-by-file-since-the-last-reviewed-head-learned-from-thread-markers--01a1cc54bd32.md)).
- **Slice 4:** in-thread `/vizzini` commands. mrman v0.1.4: `review reply` ([ADR-0019](/decisions/adr-0019-in-thread-vizzini-commands-are-answered-once-by-the-next-bot-run--01a1cc771df6.md)).
- **Slice 5:** suppression rules from the base branch, and suggested rules on dismissal ([ADR-0020](/decisions/adr-0020-suppression-rules-are-reviewed-repository-files-read-from-the-base-branch-and-applied-deterministically--01a1cca0ea51.md)).

**Still out of scope:**
- **Triggers:** the CI step is the entry point, and a webhook receiver was not built. Published CI templates followed the same day: GitHub Actions, Azure Pipelines and GitLab CI in `deploy/ci/`, with the GitHub template verified live ([ADR-0021](/decisions/adr-0021-ci-templates-install-pinned-verified-tools-the-grant-policy-is-read-from-the-base-branch--01a1ccc9f271.md)).
- **Other forges:** thread resolution and replies are implemented for Azure DevOps only. Other forges report them as unsupported until their drivers gain them.
- **Silent runs:** a run that posts nothing leaves no reviewed-head marker (ADR-0018).

## Design decided (2026-11-11): [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md)
- **Composition** (user's choice): `vizzini-bot` (Vizzini) reviews; mrman's CLI does all forge work.
- **Authorization** (user's choice): mrman `review submit --grant-policy DIR` posts without a human only when a committed garmr policy allows the forge-verified identity's event. The default is comments only, and every decision is audited.

**Slice 1 (done 2026-11-11):**
- **mrman** `feature/grant-policy` (efef0ce):
  - `review submit --grant-policy DIR` (`--grant-namespace`, `--garmr`, `--run-id`, `--agent`).
  - The `policies/garmr/mrman` `AgentSubmit` pack (MRM-001..004).
  - Fails closed.
  - Writes an audit log beside the session.
- **Vizzini** `cmd/vizzini-bot`:
  - Reviews `git diff base...head` with `--repo`, `--knowledge`, `--lint-json` and attribution.
  - Drafts through `mrman review add` with the `<!-- vizzini:fp=v1:… run=… -->` marker.
  - Submits through `--grant-policy`.
  - Exits 3 on a deny, retries mrman network errors, and has `--dry-run`.
  - Guide: [docs/guides/bot](https://infrashift.github.io/vizzini-pr-reviewer/docs/guides/bot/).
- **vizzini-lab** `make uat-bot`, live Sonnet 5.5 on ADO PR #16:
  - The committed default denied the PAT's identity (MRM-002), exit 3, nothing posted.
  - The pack listing the forge-verified identity allowed it: 9/9 findings posted as marked threads.
  - Cost $0.217 for two live runs, attributed per PR.

**Slice 5, suppression (done 2026-11-11):** [ADR-0020](/decisions/adr-0020-suppression-rules-are-reviewed-repository-files-read-from-the-base-branch-and-applied-deterministically--01a1cca0ea51.md)
- **Vizzini library** (753e529):
  - `#Suppressions` in the schema (YAML or JSON).
  - `ReviewOptions.Suppressions` moves matching findings into `ReviewOutput.Suppressed`. They are not gated, but still count against auto-approval.
  - `pr-review --suppressions`.
  - `reviewer.Fingerprint` moves into the library.
- **vizzini-bot** (d004c27):
  - Rules are read from the base revision.
  - A suppressed finding is not posted, and its thread is left alone.
  - A won't-fix or by-design dismissal gets one suggested rule.
- **vizzini-lab** (PR #30, 47/47): a dismissed thread got a suggested rule. Merged into the base branch as suggested, the rule suppressed the finding on the next full review.

**Slice 4, in-thread commands (done 2026-11-11):** [ADR-0019](/decisions/adr-0019-in-thread-vizzini-commands-are-answered-once-by-the-next-bot-run--01a1cc771df6.md)
- **mrman** `feature/thread-reply` (f13ff54):
  - `review reply --thread ID [--parent CID] BODY|--input`, gated like submit: a human grant naming `reply`, or `--grant-policy` with event `reply`.
  - `forge.ThreadReplier` and `AsThreadReplier`; Azure DevOps implements it.
  - The default pack allows `reply`.
- **Vizzini:**
  - `vizzini-bot` answers `/vizzini explain|refactor|docstrings` written in a PR thread (50bfeae). The target defaults to the thread's lines, the thread is the conversation, and a `reply-to` marker makes each command answered once.
  - Usage replies for malformed commands; `--command-authors`, `--max-commands`.
  - Two fixes the UAT forced: every bot comment records the head it reviewed (ef1cbe5), and command requests are attributed (a820a35).
  - stubllm answers `/explain` (18ca3ba).
- **vizzini-lab** `make uat-bot`, live Sonnet 5.5 on ADO PR #26, 37/37:
  - A real `/explain` got an on-point answer ($0.0139).
  - A malformed command got usage.
  - Nothing was answered twice, and the command request was attributed.

**Slice 3, incremental review (done 2026-11-11):** [ADR-0018](/decisions/adr-0018-incremental-review-by-file-since-the-last-reviewed-head-learned-from-thread-markers--01a1cc54bd32.md)
- **Vizzini** `vizzini-bot` (1eda2f4):
  - Threads are read before the review. The last reviewed head is the newest marker head that is an ancestor of the current head.
  - Only the PR's files changed since then are reviewed (`base...head` limited to them).
  - When the head has not moved, or a push only merges the base branch, no model call is made.
  - Threads on other files are carried.
  - `--full` forces a full review.
  - The JSON result gains `mode`, `since`, `reviewed_files` and `carried`.
- **vizzini-lab** `make uat-bot`, live Sonnet 5.5 on ADO PR #22, 27/27:
  - The idle leg made no review.
  - The `--full` rerun duplicated nothing.
  - The pushed fix was reviewed incrementally: one file, $0.0081 against about $0.057 for a full review. It resolved the MD5 thread and carried the 10 threads on other files.

**Slice 2, thread memory (done 2026-11-11):** [ADR-0017](/decisions/adr-0017-thread-memory-matches-by-fingerprint-or-mapped-anchor-and-resolves-only-when-the-code-changed--01a1cc2bd91a.md)
- **mrman** `feature/thread-resolve` (3e2324e, 71c0201):
  - `review threads` (read-only).
  - `review resolve --thread ID… --reply`, gated like submit: a human grant naming `resolve`, or `--grant-policy` with event `resolve`.
  - `forge.ThreadResolver` and `AsThreadResolver`.
  - Azure DevOps resolves only threads whose root comment is the token's own identity, compared by id.
  - The default pack allows `resolve`.
- **Vizzini** `vizzini-bot` (2ddfb3e):
  - The marker records head, lines and category.
  - A finding is already posted when its fingerprint matches, or its file and category match on git-mapped lines.
  - A thread is resolved only when its finding is gone and its code changed; variance keeps it open.
  - stdout is a JSON result.
- **vizzini-lab** `make uat-bot`, live Sonnet 5.5 on ADO PR #19, 21/21:
  - A same-head rerun duplicated nothing.
  - A pushed fix resolved exactly the MD5 thread, with a reply and status fixed, and kept one variance-only thread open.
  - Cost $0.22 for four reviews.
- **Toward the criteria below:**
  - *No duplicate threads on push* is verified, though the re-review is still a full review, not incremental (slice 3).
  - *Thread resolution when fixed* is verified.
  - Suppression is partial: a thread a person resolved stays resolved and is not reposted. Formal suppression rules are slice 5.

**Learned in slice 1:**
- dev.azure.com intermittently resets TLS handshakes, so forge calls need retries.
- In a reused workspace, a denied run's drafts stay in the mrman session. CI workspaces are fresh, but slice 2's dedupe should not count on that: it should match against the forge's threads and ignore local drafts.
- On ADO, the identity mrman verifies is the token owner's display name. A real deployment needs a dedicated bot account, or the pipeline's build-service identity, listed in the policy.
