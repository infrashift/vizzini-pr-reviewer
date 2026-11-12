---
id: "01a1cf8f-51ab-7942-9c07-146aedc9c731"
title: "ISSUE-026: Webhook receiver answers /vizzini commands immediately (experimental)"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "automation"
  - "webhook"
  - "experimental"
  - "open"
created: "2026-11-12T05:40:38-06:00"
timestamp: "2026-11-12T05:40:38-06:00"
description: "An optional, experimental receiver for Azure DevOps service hooks that answers /vizzini commands as they are written, by running vizzini-bot --commands-only, instead of waiting for the PR's next CI run."
---

# ISSUE-026: Webhook receiver answers /vizzini commands immediately (experimental)

**Status:** open, experimental · **Raised:** 2026-11-12 · **Decision:** [ADR-0022](/decisions/adr-0022-commands-are-answered-from-a-forge-webhook-by-a-commands-only-bot-run--01a1cf8f51be.md) · **Follows:** [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 4

## Goal
A `/vizzini` command is answered by the next bot run ([ADR-0019](/decisions/adr-0019-in-thread-vizzini-commands-are-answered-once-by-the-next-bot-run--01a1cc771df6.md)), and in CI the bot runs on a push. A comment starts no pipeline, so a question waits for the next push. The goal is an **optional** receiver that answers commands as soon as they are written, without changing anything for teams that do not deploy it.

## Acceptance criteria
- [x] `vizzini-bot --commands-only`: answers pending commands; no review, no resolution, no suggestion; refuses `--dry-run` and `--max-commands 0`.
- [x] `cmd/vizzini-webhook`: standard library only; `POST /hooks/azure-devops`, `GET /healthz`.
- [x] Deliveries authenticated with basic auth (constant time). Payloads limited to 1 MiB. The repository is allowlisted, and the target branch is validated as a plain branch name.
- [x] Only a text comment with a `/vizzini` line, on an active PR, not written by the bot, is acted on; everything else gets `200` with a reason.
- [x] One job per PR: deliveries fold into a waiting job; a delivery during a run causes one more run.
- [x] Jobs fetch `refs/pull/<id>/merge` into a cached clone, check out its second parent, and run the bot with the same attribution as CI (`--pr`, `--requester`). The token reaches git through a credential helper, never argv.
- [x] A policy denial logs the identity to allow.
- [x] Unit tests against Azure DevOps' own sample event (from its test-notification API).
- [x] Admin runbook and deployment kit: `deploy/webhook/` (README runbook, hardened systemd unit, env templates, Caddy example, `ado-subscription.sh`). Docs site guide *Webhook Receiver (Experimental)*.
- [x] Live UAT in vizzini-uat (suite TS-HOOK): a real ADO PR, the receiver on this machine, a delivery in the service hook's shape built from the real comment and PR, and a reply in the thread with no push.
- [ ] A live delivery **by Azure DevOps itself** to a public endpoint. This needs a public HTTPS host; not done.
- [ ] GitHub (`issue_comment`, HMAC signature) and GitLab (note hook, `X-Gitlab-Token`): blocked until mrman can reply on those forges.
- [ ] Leave experimental: run it in real use; decide on the double-answer race (below).

## Known limits
- **Double answers:** a CI run and a receiver job that start together can both answer one command. A per-PR lock across CI and the receiver would need shared state; the experiment accepts the rare duplicate.
- **Legacy hosts:** `*.visualstudio.com` organizations are refused; the receiver builds clone URLs for `dev.azure.com` only.
- **Restarts:** a restart drops waiting jobs. The next comment, or the next CI run, answers their commands.
