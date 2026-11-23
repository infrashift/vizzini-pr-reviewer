---
x-okfw-id: "01a20074-08e3-753f-8fc5-3d26364fa107"
title: "ISSUE-035: install.sh does not retry a reset download"
type: "issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "ci"
  - "reliability"
  - "closed"
x-okfw-created: "2026-11-21T17:32:14-06:00"
x-okfw-timestamp: "2026-11-21T17:32:14-06:00"
description: "A reset connection while Go fetched garmr's toolchain failed a CI job; install.sh now retries the go steps, curl retries any error, and v0.1.2 ships it to the templates."
---

# ISSUE-035: install.sh does not retry a reset download


**Status:** closed (released in v0.1.2; the templates pin it) · **Raised:** 2026-11-21 · **Related:** [ISSUE-034](/issues/issue-034-use-the-v0-1-0-release-downstream--01a1fff0f982.md), [2026-11-21 daily note](/daily/2026/11/2026-11-21-saturday--01a200662be1.md)

## Problem
vizzini-lab's GitLab template suite failed once, on run 20261121T225224Z, job 2. Go reset its connection to proxy.golang.org while it downloaded the go1.26.11 toolchain that garmr requires. `install.sh` ran `go install` once, so the job failed before mrman was installed. A re-run passed.

`curl --retry 3` does not cover this case either. It retries timeouts and HTTP 408, 429 and 5xx, but not a connection reset (curl exit 56).

## Fix
- **Go steps:** a `retry` function runs `go install` (garmr) and the from-source `go build` up to three times, 10 s and then 20 s apart. It logs each failed attempt and returns non-zero after the third.
- **curl:** `install.sh` and `scripts/tiktoken-cache.sh` add `--retry-all-errors`, which needs curl 7.71 or newer. The checksum checks still catch a bad download.
- **Release:** the templates run `install.sh` from the release they pin, so v0.1.2 ships the fix, and the templates pin v0.1.2.

## Acceptance criteria
- [x] shellcheck passes. A fake `go` that fails its first `install` call is retried once and the install completes. A command that always fails stops after three attempts with exit 1.
- [x] The CI README and the bot guide say that downloads are retried.
- [x] v0.1.2 released (run 44962299434; checksums and attestations checked); the templates pin it; `make ci-check` passes.
- [x] vizzini-lab's three template suites pass live on v0.1.2.

## Resolution
v0.1.2 (`bfe7f12`) ships the retries, and the templates pin it (`a65f2cf`). vizzini-lab pins `a65f2cf` with mrman v0.1.10 and ran every suite that uses either pin, on stubllm, on 2026-11-21. All pass:
- the three template suites: GitHub 16/16, Azure Pipelines 14/14, GitLab 17/17;
- `uat-stub` 36/36, `uat-bot-stub` 46/46, `uat-workflow` 26/26, `uat-webhook` 30/30.

No job needed a retry on these runs. The local test above shows the retry recovering. Record: vizzini-lab `results/2026-11-21-pins-vizzini-v012-mrman-v0110.md`.
