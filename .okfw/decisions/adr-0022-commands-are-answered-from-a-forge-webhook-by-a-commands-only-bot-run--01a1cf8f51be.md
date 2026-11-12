---
id: "01a1cf8f-51be-792d-a004-a82560f61243"
title: "ADR-0022: Commands are answered from a forge webhook by a commands-only bot run"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "webhook"
  - "experimental"
created: "2026-11-12T05:40:38-06:00"
timestamp: "2026-11-12T05:40:38-06:00"
description: "An optional receiver turns Azure DevOps comment events into vizzini-bot --commands-only runs: replies only, authorized by the base branch's policy, with reviews left to CI."
---

# ADR-0022: Commands are answered from a forge webhook by a commands-only bot run

**Status:** Accepted (experimental)
**Date:** 2026-11-12
**Source:** [ISSUE-026](/issues/issue-026-webhook-receiver-answers-vizzini-commands-immediately-experimental--01a1cf8f51ab.md)
**Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md) (posting by composition, the policy is the grant), [ADR-0019](/decisions/adr-0019-in-thread-vizzini-commands-are-answered-once-by-the-next-bot-run--01a1cc771df6.md) (commands answered once by the next run), [ADR-0021](/decisions/adr-0021-ci-templates-install-pinned-verified-tools-the-grant-policy-is-read-from-the-base-branch--01a1ccc9f271.md) (CI templates)

## Context
Commands are answered by the next vizzini-bot run, and a comment starts no CI run. A user who writes `/vizzini explain` therefore waits for the next push. Every forge can call a webhook on a new comment. The question was what such a call should run, and with what authority.

## Decision
- **An optional receiver, not a change to CI.** `cmd/vizzini-webhook` is a separate binary, off unless an admin deploys it. The CI templates and the bot's default behavior do not change.
- **It runs the bot, commands only.** On a qualifying comment, it runs `vizzini-bot --commands-only` on the PR. That mode answers pending commands and does nothing else: no review, no resolution, no suggestions.
  - A comment can never trigger a paid re-review.
  - It cannot race CI over review threads; reviews stay with the pipeline.
  - Every safeguard of the bot still applies: the grant policy is read from the base branch, garmr authorizes event `reply` for the receiver's identity, and each command is answered once (the `reply-to` marker).
- **It does not trigger the pipeline.** Queuing a CI run per comment would also run a review, and would wait for an agent and a tool install. Running the bot directly answers within about a minute.
- **A dedicated identity.** The receiver posts as its own bot account (a PAT, *Code (Read & Write)*). The policy must list it next to the CI identity, so allowing replies from the receiver is a reviewed change.
- **Azure DevOps first.** ADO service hooks deliver *Pull request commented on* with basic auth over HTTPS. GitHub and GitLab wait for mrman's reply support.
- **Admission before work.** The receiver checks credentials in constant time, limits the body size, allowlists repositories and validates branch names. It ignores bot-written comments (the `<!-- vizzini:` marker) and inactive PRs, answers `202` at once, and runs one job per PR with coalescing.
- **Experimental.** It starts with a warning, and its flags and files may change. It leaves experimental only through a recorded decision ([ISSUE-026](/issues/issue-026-webhook-receiver-answers-vizzini-commands-immediately-experimental--01a1cf8f51ab.md)).

## Consequences
- **Commands answered without a push.** The vizzini-uat suite TS-HOOK shows this on a real ADO PR, with the receiver delivered to locally.
- **A new public endpoint to run.** It needs HTTPS, a secret and a token, and an admin follows `deploy/webhook/README.md` to deploy it.
- **Rare double answers** if a CI run and a receiver job overlap; accepted for the experiment.
- **Comment authors can spend model calls.** `--command-authors`, `--max-commands` and the LiteLLM key's budget bound it.
