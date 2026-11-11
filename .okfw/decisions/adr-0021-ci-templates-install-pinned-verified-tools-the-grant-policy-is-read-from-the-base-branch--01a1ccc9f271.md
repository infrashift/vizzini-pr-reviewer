---
id: "01a1ccc9-f271-7be2-a04b-014e802cf3f6"
title: "ADR-0021: CI templates install pinned, verified tools; the grant policy is read from the base branch"
type: "ADR"
tags:
  - "adr"
  - "automation"
  - "ci"
  - "security"
created: "2026-11-11T16:45:49-06:00"
timestamp: "2026-11-11T16:45:49-06:00"
description: "vizzini-bot ships GitHub Actions, Azure Pipelines and GitLab CI templates that install pinned, checksum-verified tools and post under a policy read from the base branch, with no identity allowed until the first, denied run names it."
---

# ADR-0021: CI templates install pinned, verified tools; the grant policy is read from the base branch


**Status:** Accepted
**Date:** 2026-11-11
**Source:** follow-up to [ISSUE-024](/issues/issue-024-automation-mode-review-every-pr-incrementally-with-thread-memory--01a1cba3cab7.md) (published CI templates)
**Related:** [ADR-0016](/decisions/adr-0016-automation-mode-by-composition-vizzini-bot-mrmans-forge-cli-garmr-authorized-posting--01a1cbedb12e.md) (the policy is the grant), [ADR-0020](/decisions/adr-0020-suppression-rules-are-reviewed-repository-files-read-from-the-base-branch-and-applied-deterministically--01a1cca0ea51.md) (rules from the base branch)

## Context
vizzini-bot needs three binaries (itself, mrman, garmr) plus the tokenizer encodings in every CI job. Vizzini and garmr have no releases yet, and mrman has. The templates also forced a question that ADR-0016 had left implicit: where does the grant policy come from? The bot passed `--grant-policy` as a path in the pull request's checkout, so a PR could edit the policy it is judged by, for example to allow `approve`, and have that edit authorize its own posting.

## Decision
- **One template per platform** in `deploy/ci/`: GitHub Actions (`pull_request`, same-repository PRs only, actions pinned by commit, untrusted values passed through `env`), Azure Pipelines (build validation, `System.AccessToken`) and GitLab CI (merge-request pipelines, a project access token). Each checks out with full history and runs the bot.
- **`install.sh` installs pinned, verified tools:**
  - `vizzini-bot`, built from a pinned Vizzini commit;
  - garmr at a pinned commit (`go install`);
  - the pinned mrman release, checked against its `SHA256SUMS`;
  - the tokenizer encodings, checked against pinned SHA-256 sums.

  It sets `GOTOOLCHAIN=auto`, because garmr needs a newer Go than CI images ship with. The toolchain go fetches is verified against Go's checksum database.
- **The grant policy is read from the base branch.** A relative `--grant-policy` is copied out of the base revision (`git ls-tree` / `git show`). Only a reviewed, merged policy change takes effect, and a policy missing from the base fails the run. An absolute path is the operator's own and is used as it is.
- **No identity by default.** The starter policy (mrman's pack) allows no identity, so the first run is denied (exit 3), and its log names the identity the CI posts as. Allowing it is a reviewed change on the default branch.

## Consequences
- **Verified live on GitHub** (vizzini-uat, `make uat-ci-github`, 13/13), with stubllm as the model in the job:
  - the onboarding sequence: denied run, identity, allowed re-run posting comments as `github-actions[bot]`;
  - an incremental second run with no duplicates.

  The first attempt found the Go toolchain problem that `GOTOOLCHAIN=auto` fixes.
- **Not run live:**
  - Azure Pipelines: the test organization has no hosted parallelism for private projects; a self-hosted agent would do.
  - GitLab CI: no project was available.

  Their scripts pass shellcheck and their YAML parses, and the shared install step is verified.
- **Pin bumps:** the templates pin Vizzini, mrman and garmr. Bumping a pin is a reviewed change in the adopting repository.
