---
id: "01a1cca0-ea51-7ce8-a6a5-6477c461de8e"
title: "ADR-0020: Suppression rules are reviewed repository files, read from the base branch and applied deterministically"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "suppression"
created: "2026-11-11T16:01:00-06:00"
timestamp: "2026-11-11T16:01:00-06:00"
description: "Findings a team decided not to see again are recorded as CUE-validated rules in .vizzini/suppressions.yaml, read by vizzini-bot from the base revision and applied after the model answers; a dismissal earns a suggested rule, never an automatic one."
---

# ADR-0020: Suppression rules are reviewed repository files, read from the base branch and applied deterministically


**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) slice 5
**Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md) (the policy is the grant), [ADR-0017](/decisions/adr-0017-thread-memory-matches-by-fingerprint-or-mapped-anchor-and-resolves-only-when-the-code-changed--01a1cc2bd91a.md) (fingerprints)

## Context
ADR-0017 already keeps a dismissal within one pull request: a thread a person resolved stays resolved, and the same finding is not reposted while its code stands still. A team also wants a dismissal to hold across pull requests and code changes ("we know; stop telling us"). It has to be explicit and reviewable, and an author must not be able to use it to silence findings on their own change.

## Decision
- **Rules are a repository file,** `.vizzini/suppressions.yaml` (YAML or JSON), validated against `#Suppressions` in `schema/review.cue`.
  - A rule has an `id` and a `reason`, and names at least one of `fingerprint` (the v1 recipe, now `reviewer.Fingerprint`), `paths` (globs) or `title` (a case-insensitive substring).
  - `categories` and `expires` narrow it further.
  - Ids are unique. An invalid file fails the run rather than being silently ignored.
- **Deterministic, not advisory.** ISSUE-024 imagined feeding rules back to the model as a knowledge-style document. A prompt hint binds nothing and costs tokens on every run, so rules are instead applied to the validated findings after the model answers.
  - `ReviewOptions.Suppressions` moves a matching finding into `ReviewOutput.Suppressed`, with its rule id and reason.
  - That list appears only in the closed `#ReviewReport`, so the model cannot write into it.
  - `pr-review --suppressions` uses the same path.
- **What suppression changes:**
  - **Changed:** a suppressed finding is not posted and does not fail the `--fail-on` gate.
  - **Unchanged:** it **still counts against auto-approval**, so a rule decides what is posted, never that a human may be skipped. A thread a suppressed finding matches is left alone.
- **Read from the base revision.** vizzini-bot reads the file with `git show <base>:<path>`. A rule takes effect once it is merged, so it goes through review like the grant policy does. A rule added in the pull request itself has no effect on that pull request.
- **Dismissal suggests a rule; it never makes one.** When a person resolves one of the bot's threads as *won't fix* or *by design*, the bot replies once with a ready-to-commit rule covering that file and category, with the finding's title as a narrower, commented-out alternative.
  - The reply goes through `mrman review reply` (event `reply`) and carries a `suggested-suppression` marker.
  - Committing the rule is a person's decision.

## Consequences
- **Breadth versus robustness.** A suggested `paths` plus `categories` rule is broad (every finding of that category in that file), but it survives a model rewording the title. The fingerprint or title forms are narrower but more fragile; the suggestion says so.
- **Verified in vizzini-lab** (ADR-0005, PR #30, 47/47). The bot suggested a rule for a dismissed thread. Merged into the base branch exactly as suggested, the rule suppressed the finding on the next full review: nothing was posted, and no second suggestion was made.
