---
id: "01a20074-08e3-753f-8fc5-3d26364fa107"
title: "ISSUE-035: install.sh does not retry a reset download"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "ci"
  - "reliability"
  - "in-progress"
created: "2026-11-21T17:32:14-06:00"
timestamp: "2026-11-21T17:32:14-06:00"
description: "A reset connection while Go fetched garmr's toolchain failed a CI job; install.sh now retries the go steps, curl retries any error, and v0.1.2 ships it to the templates."
---

# ISSUE-035: install.sh does not retry a reset download


**Status:** in-progress · **Raised:** 2026-11-21 · **Related:** [ISSUE-034](/issues/issue-034-use-the-v0-1-0-release-downstream--01a1fff0f982.md), [2026-11-21 daily note](/daily/2026/11/2026-11-21-saturday--01a200662be1.md)

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
- [ ] v0.1.2 released; the templates pin it; `make ci-check` passes.
- [ ] vizzini-lab's three template suites pass live on v0.1.2.
