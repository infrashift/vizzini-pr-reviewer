---
x-okfw-id: "01a205a9-7301-7c0a-9a6e-fa09658727ba"
title: "ISSUE-036: install.sh downloads every tool on every job, also on a self-hosted agent"
type: "issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "ci"
  - "reliability"
  - "closed"
x-okfw-created: "2026-11-22T17:48:40-06:00"
x-okfw-timestamp: "2026-11-22T17:48:40-06:00"
description: "Every job on a persistent self-hosted agent downloads and verifies the same pinned tools again; install.sh now keeps them in a cache under their pins."
---

# ISSUE-036: install.sh downloads every tool on every job, also on a self-hosted agent

**Status:** closed (released in v0.1.3; the templates pin it) · **Raised:** 2026-11-22 · **Related:** [ADR-0026](/decisions/adr-0026-install-sh-keeps-verified-tools-in-a-cache-under-their-pins--01a205a972f2.md), [ISSUE-035](/issues/issue-035-install-sh-does-not-retry-a-reset-download--01a2007408e3.md), [2026-11-22 daily note](/daily/2026/11/2026-11-22-sunday--01a2008d7400.md)

## Problem
The CI templates run `install.sh` in every job. It downloads `vizzini-bot`, mrman and the tokenizer encodings, builds garmr with `go install` (which also fetches the Go toolchain garmr requires), and verifies each one. On a hosted runner this is necessary. On a persistent self-hosted agent, every job repeats the same work for the same pins, about 45 s, and each download can fail the job.

## Fix
- `install.sh` keeps each verified binary in `VIZZINI_TOOLS_CACHE` under its pin, and later jobs copy it from there ([ADR-0026](/decisions/adr-0026-install-sh-keeps-verified-tools-in-a-cache-under-their-pins--01a205a972f2.md)). It uses an entry only while its binary matches the SHA-256 recorded with it, and replaces a damaged entry.
- The three templates set `VIZZINI_TOOLS_CACHE` to a directory that stays between jobs on a self-hosted agent.
- `deploy/ci/README.md` has a *Tools cache* section, and the bot guide mentions it.
- garmr v0.1.0 is released, so `install.sh` downloads it like mrman, and the templates need no Go ([ADR-0027]({ADR27})).

## Acceptance criteria
- [x] shellcheck and `make ci-check` pass.
- [x] Local test (v0.1.2 commit, this `install.sh`): a cold cache installs everything (44 s); a warm cache installs from the cache (0.3 s) and the commit check still runs; a damaged mrman or garmr entry is replaced; another mrman pin gets its own entry; two cold jobs at the same time both pass and leave whole entries and no temporary directories; without the variable, nothing changes; a cached `vizzini-bot` with a checkout at another commit stops with the commit error.
- [x] garmr from its v0.1.0 release. Tested in `buildpack-deps:bookworm-scm` (no Go): a cold cache downloads and checks all three releases; a warm cache takes all three from the cache. The source build of `vizzini-bot` still works.
- [x] v0.1.3 released (run 45087867085; checksums and one attestation per archive checked; the binaries have no code changes since v0.1.2). The templates pin it.
- [x] vizzini-lab's three template suites pass live. The Azure suite runs three jobs on one agent, and jobs 2 and 3 install all three tools from the cache.

## Resolution
v0.1.3 (`ebb7322`) ships the cache and the garmr release install. Its binaries have no code changes since v0.1.2. The templates pin it (`f6e27e3`). vizzini-lab pins `f6e27e3` and ran its template suites live on 2026-11-22, on stubllm:
- **GitHub 17/17, Azure 17/17, GitLab 18/18.** Each first job installed the v0.1.3 `vizzini-bot`, garmr v0.1.0 and mrman v0.1.10 from their releases.
- **Azure (self-hosted agent):** build 1 found the cache empty, and its install step took 2 min 18 s. Builds 2 and 3 installed all three tools from the cache in under 0.3 s.
- **GitLab (docker executor):** jobs 2 and 3 used the cache too, because the executor keeps the builds volume between jobs.

On a warm agent, cloning Vizzini is now most of the install step: [ISSUE-037](/issues/issue-037-the-ci-templates-clone-the-whole-vizzini-repository-on-every-job--01a205d331aa.md). Record: vizzini-lab `results/2026-11-22-vizzini-v013-tools-cache.md`.

