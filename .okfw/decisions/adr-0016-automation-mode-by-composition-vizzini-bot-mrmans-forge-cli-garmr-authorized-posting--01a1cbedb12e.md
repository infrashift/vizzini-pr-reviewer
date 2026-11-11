---
id: "01a1cbed-b12e-7f08-bb18-3702c02f4119"
title: "ADR-0016: Automation mode by composition: vizzini-bot, mrman's forge CLI, garmr-authorized posting"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "forge"
  - "mrman"
  - "garmr"
  - "security"
created: "2026-11-11T12:45:14-06:00"
timestamp: "2026-11-11T12:45:14-06:00"
description: "Unattended PR review is a thin vizzini-bot orchestrator that reviews with the Vizzini library and does all forge work through mrman's CLI; mrman submits without a human only when a committed garmr policy allows the forge-verified bot identity's action, and every decision is audited."
---

# ADR-0016: Automation mode by composition: vizzini-bot, mrman's forge CLI, garmr-authorized posting

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md); both choices made by the user
**Related:** [ADR-0010](/decisions/adr-0010-json-reports-carry-an-apiversion-kind-envelope-garmr-consumes-them--01a1ca09b067.md) (garmr), [ADR-0015](/decisions/adr-0015-cost-attribution-through-litellm-spend-logs-correlated-by-run-and-pull-request--01a1cbc7d120.md) (attribution)

## Context
Open-source reviewers install as bots: they review every PR on open and push, remember their threads, and answer commands in the PR. Vizzini is a stateless library and CLI with no forge code. mrman already has forge drivers (GitHub, GitLab, Azure DevOps, Forgejo), including the commit-range diff, review-thread listing and review creation a bot needs, but they are internal Go packages. mrman also promises it never calls a model, and its agent-submit interlock grants submit only to a human at an interactive terminal (`--auto`), so that an agent cannot authorize itself.

## Decision
- **Composition over duplication.**
  - A thin `vizzini-bot` command (in Vizzini, `cmd/vizzini-bot`) reviews with the Vizzini library and does **all** forge work through mrman's CLI: open the PR headlessly, list threads, add comments, submit, resolve.
  - Vizzini stays forge-agnostic and mrman stays model-free. Forge support grows in one place, mrman's drivers.
- **Policy-authorized posting** (the human grant's unattended counterpart):
  - `mrman review submit --grant-policy DIR` submits without a TTY grant only when garmr allows it. garmr evaluates the committed policy in-process (`garmr eval --policy-dir`) against an `mrman.infrashift.io/v1` `AgentSubmit` input: the event, the **forge-verified** identity (the token's own login), the PR, the comment count, the run ID and the agent.
  - The default pack allows only `comment`, only for listed bot identities, and only up to a comment cap. `approve` and `request-changes` are denied unless a reviewed policy change lists them.
  - mrman still never grants itself anything: the human review of the policy file *is* the grant. A deny or an evaluation error refuses the submit.
  - Every decision (allow, warn or deny, with failed rules, policy digest and run ID) is recorded in the session and printed as JSON.
- **The diff comes from the CI checkout:** `git diff base...head`, as CI already has the repository. That also gives `--repo` context (ADR-0014). Attribution is the real PR key, head SHA and pipeline identity (ADR-0015).
- **Thread memory lives in the forge, not in a store:** each posted finding carries a hidden fingerprint marker (`<!-- vizzini:fp=… -->`). Re-runs list the threads to skip findings already posted and to resolve threads whose finding is gone. No database is needed.
- **Slices:**
  1. Policy-authorized submit, plus `vizzini-bot` posting a review on an ADO PR from CI.
  2. Thread memory: dedupe and resolve.
  3. Incremental review of the commit range since the last reviewed head.
  4. In-thread `/vizzini` commands.
  5. Feedback suppression from dismissed findings.

## Consequences
- **Two binaries in CI** (vizzini-bot and mrman, plus garmr for the policy), each with one job.
- **mrman's security posture changes** from "human only" to "human or committed policy". That has to stay explicit: off unless `--grant-policy` is passed. With it, the policy alone decides and any TTY grant on the session is ignored. Every decision is audited.
- **Forge mutations mrman lacks** (resolving or replying to a thread) are added to its drivers as later slices need them.
