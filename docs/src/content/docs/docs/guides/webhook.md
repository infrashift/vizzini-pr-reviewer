---
title: Webhook Receiver (Experimental)
description: An optional receiver that answers /vizzini commands on Azure DevOps pull requests as soon as they are written, instead of on the next CI run.
---

:::caution[Experimental and optional]
`vizzini-webhook` is an experiment. Without it, nothing changes: the CI
templates review every push, and a `/vizzini` command is answered by the
next pipeline run. It supports **Azure DevOps Services only**, and its
flags and files can change between releases.
:::

## Why
In CI, [vizzini-bot](../bot/) runs when the
pipeline runs, and a pipeline runs on a push. A comment does not start one,
so a `/vizzini explain` waits until the next push or a manual re-run. The
receiver closes that gap: Azure DevOps tells it about each comment, and it
answers within a minute or so.

## How it works
1. **Delivery.** An Azure DevOps service hook (*Pull request commented on*,
   Web Hooks) posts each new comment to `https://HOST/hooks/azure-devops`,
   with HTTP basic auth.
2. **Filter.** The receiver acts only on a text comment with a `/vizzini`
   line, on an active pull request of an allowlisted repository, that the
   bot did not write. It answers `202` at once.
3. **Queue.** One job per pull request: comments that arrive while a job
   waits join it, and a comment during a running job makes it run once more.
4. **Answer.** The job fetches the PR into a cached clone and runs
   `vizzini-bot --commands-only`, which answers pending commands and does
   **nothing else**: no review, no resolved threads, no suggestions. Reviews
   stay with CI.
5. **Authorization is unchanged.** mrman posts a reply only if the grant
   policy on the **base branch** allows event `reply` for the receiver's
   identity, a dedicated bot account. Each command is still answered once.

## `vizzini-bot --commands-only`
The receiver's mode is a vizzini-bot flag, usable on its own:

```sh
vizzini-bot --repo . --base origin/main --head "$HEAD" \
  --pr-url "$PR_URL" --grant-policy .vizzini/grant-policy --commands-only
```

The run's JSON result has `"mode":"commands"`, `answered` and
`command_failures`. It refuses `--dry-run` and `--max-commands 0`.

## Deploy it
The runbook is in the repository: **[`deploy/webhook/README.md`](https://github.com/infrashift/vizzini-pr-reviewer/blob/main/deploy/webhook/README.md)**.
It takes an administrator from a bare host to a verified reply: install,
secrets, a hardened systemd unit, TLS, the grant policy, the service hook
subscription (script or UI), verification, operations and troubleshooting.

## Limits
- **Azure DevOps only.** mrman cannot reply in GitHub or GitLab threads yet.
  Organizations on legacy `*.visualstudio.com` URLs are not supported.
- **Public HTTPS required.** Azure DevOps does not deliver to localhost,
  private addresses or plain HTTP.
- **Rare double answers.** A CI run and a receiver job that start at the same
  moment can both answer one command.
- **Cost.** Anyone who can comment can spend a model call. Bound it with
  `--command-authors`, `--max-commands` and the LiteLLM key's budget.
