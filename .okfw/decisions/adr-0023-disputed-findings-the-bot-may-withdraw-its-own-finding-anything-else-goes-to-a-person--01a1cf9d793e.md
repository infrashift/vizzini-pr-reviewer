---
id: "01a1cf9d-793e-7781-8d31-0c4e2243b060"
title: "ADR-0023: Disputed findings: the bot may withdraw its own finding; anything else goes to a person"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "commands"
  - "agents"
created: "2026-11-12T05:56:06-06:00"
timestamp: "2026-11-12T05:56:06-06:00"
description: "/vizzini reconsider lets the bot re-evaluate its finding: a withdrawal resolves only the bot's own thread, a finding that stands stays open for a person, and a developer's identity can never resolve the bot's threads (mrman's own-thread rule)."
---

# ADR-0023: Disputed findings: the bot may withdraw its own finding; anything else goes to a person

**Status:** Accepted
**Date:** 2026-11-12
**Source:** [ISSUE-029](/issues/issue-029-vizzini-reconsider-the-bot-re-evaluates-a-disputed-finding--01a1cf97e0c1.md); with [ISSUE-027](/issues/issue-027-mrman-findings-view-for-agents-review-threads-findings--01a1cf97e098.md) and [ISSUE-028](/issues/issue-028-mrman-reply-and-resolve-on-github-and-gitlab--01a1cf97e0b0.md)
**Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md) (the policy is the grant), [ADR-0019](/decisions/adr-0019-in-thread-vizzini-commands-are-answered-once-by-the-next-bot-run--01a1cc771df6.md) (commands)

## Context
A developer, a person or an agent, either fixes a finding or argues that it is wrong. Fixes need no decision: the next run sees the code under the thread change and the finding gone, and resolves the thread itself. Disagreements need a rule for who settles them. If the developer could close the bot's threads, an agent would grade its own work. If the bot always held its ground, false positives would pile up.

## Decision
- **The bot may withdraw only its own findings.** `/vizzini reconsider [argument]`, valid only in a thread the bot opened for a finding, makes the model re-evaluate that finding against the code and the thread. Its answer, `#Reconsideration` (`verdict`: `withdrawn` or `stands`, plus an explanation), is checked against the CUE schema.
- **Withdrawn:** the bot resolves its own thread through `mrman review resolve`, with the explanation as the reply, under event `resolve` of the base-branch policy. Reconciliation already treats a resolved thread whose code has not changed as raised, so the finding is not posted again. It is posted as a regression only if its code changes and the finding comes back.
- **Stands:** the bot replies and leaves the thread **open**. A person decides: resolving as *won't fix* or *by design* starts the suppression path (ADR-0020). The prompt says that an assertion without evidence does not withdraw a finding.
- **Developers never resolve the bot's threads.** This needs no new rule: mrman resolves only threads its token's identity started. A developer agent with its **own** account (required) can reply, but cannot close vizzini-bot's threads. Its policy lists only `reply`, perhaps `comment`, and never `approve`.
- **Answered once:** a reconsider is a command like the others (the `reply-to` marker, `--command-authors`, `--max-commands`, the webhook receiver).

## Consequences
- **Two models can disagree without deadlock.** The developer's model argues, Vizzini's model re-evaluates, and an unresolved dispute waits for a person.
- **The suppression path is the only way a finding that stands goes away.** That path is a reviewed merge to the base branch.
- **The withdrawal is recorded in the thread,** with the model's reasons, for a person to audit.
