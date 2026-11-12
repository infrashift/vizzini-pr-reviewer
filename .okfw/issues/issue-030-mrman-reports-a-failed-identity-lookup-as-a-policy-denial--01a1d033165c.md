---
id: "01a1d033-165c-7dea-9818-8dfe2c93956c"
title: "ISSUE-030: mrman reports a failed identity lookup as a policy denial"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "mrman"
  - "reliability"
  - "closed"
created: "2026-11-12T08:39:31-06:00"
timestamp: "2026-11-12T08:39:31-06:00"
description: "When mrman cannot ask the forge who its token is, it evaluated the grant policy with an empty identity and reported a policy denial that nothing retries; it should return the forge's (retryable) error and evaluate nothing."
---

# ISSUE-030: mrman reports a failed identity lookup as a policy denial

**Status:** closed (released in mrman v0.1.8) · **Raised:** 2026-11-12 · **Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md), [ISSUE-026](/issues/issue-026-webhook-receiver-answers-vizzini-commands-immediately-experimental--01a1cf8f51ab.md)

## Problem
Before an unattended submit, resolve or reply, mrman asks the forge who the token is, and garmr then evaluates the grant policy for that identity. In `authorizeAction`, a failure of that call was swallowed. The policy was evaluated with an **empty identity**, so it denied with MRM-002.

**How it fails:**
- It fails closed, so it is safe. But the caller sees a **policy denial** (vizzini-bot exit 3) that names no identity, for what was really a transient network error.
- vizzini-bot retries mrman calls that fail with "network error", but it never retries a denial. The action is lost until the next run.
- The audit log records a decision that was never really made.

**Seen live:** vizzini-lab's webhook suite on mrman v0.1.7 (run 20261112T141226Z, 29/30). The receiver's job for one command was denied after a connection reset, and the log said "list this receiver's identity" with no identity to list. A re-run passed 30/30. Earlier, the full-workflow suite saw the same empty identity on an approve attempt, where the event rule also denied it.

## Fix
- **On a failed lookup,** `authorizeAction` returns the forge's error and evaluates nothing, audits nothing and posts nothing. A network error keeps its kind (`forge.ErrorNetwork`), so vizzini-bot retries it.
- **A forge that answers without a login** is still evaluated as it is: that is an answer, not a failure.
- **Test:** `TestResolveByPolicy_IdentityLookupFails` checks that the error is not a denial, that garmr is not called, and that there is no audit record and no resolve.

## Acceptance criteria
- [x] The fix and the test; mrman `make check` passes (87.5% coverage).
- [x] The agents guide says what happens.
- [x] Merged (mrman PR #19) and released in v0.1.8, pinned in the CI templates (Vizzini `1ce20a9`) and vizzini-lab.
- [x] The vizzini-lab webhook (30/30) and workflow (26/26) suites passed on v0.1.8, with the rest of the batch (results/2026-11-12-live-mrman-v018.md).
