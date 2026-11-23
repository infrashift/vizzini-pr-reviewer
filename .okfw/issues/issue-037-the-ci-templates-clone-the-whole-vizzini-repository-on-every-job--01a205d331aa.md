---
id: "01a205d3-31aa-7958-a98e-71bf88df0f3b"
title: "ISSUE-037: The CI templates clone the whole Vizzini repository on every job"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "ci"
  - "performance"
  - "closed"
created: "2026-11-22T18:34:16-06:00"
timestamp: "2026-11-22T18:34:16-06:00"
description: "On a warm self-hosted agent the tools install in under a second, but the job still clones Vizzini's full history to run install.sh, 11 to 39 s in vizzini-lab; a shallow fetch of the pinned commit would do."
---

# ISSUE-037: The CI templates clone the whole Vizzini repository on every job

**Status:** closed (templates changed in `54b211a`; no release needed) · **Raised:** 2026-11-22 · **Related:** [ISSUE-036](/issues/issue-036-install-sh-downloads-every-tool-on-every-job-also-on-a-self-hosted-agent--01a205a97301.md), [ADR-0026](/decisions/adr-0026-install-sh-keeps-verified-tools-in-a-cache-under-their-pins--01a205a972f2.md), [2026-11-22 daily note](/daily/2026/11/2026-11-22-sunday--01a2008d7400.md)

## Problem
Each template clones `infrashift/vizzini-pr-reviewer` with its full history (`git clone --no-checkout`, or `actions/checkout` on GitHub) only to run `deploy/ci/install.sh` and `scripts/tiktoken-cache.sh` from the pinned commit. In vizzini-lab's Azure suite on 2026-11-22 (run 20261123T002004Z), builds 2 and 3 installed every tool from the cache in under 0.3 s, but the clone before it took 39 s and 11 s. The clone is now most of the install step on a warm agent, and it needs the network.

## Finding: the history is not the cost
Measured from a workstation on 2026-11-22, at the v0.1.3 commit:

| Fetch | `.git` size | Time (3 runs) |
|---|---|---|
| full clone | 4.4 MB | 51 s, 27 s, 19 s |
| depth 1 | 4.1 MB | 28 s, 23 s, 13 s |
| depth 1, `--filter=blob:none`, sparse `deploy/ci` and `scripts` | 264 KB | 6.1 s, 1.5 s, 1.3 s |

The history is small. One file, `docs/src/assets/vizzini.jpeg` (3.4 MB), is most of every commit. The job needs only `deploy/ci/install.sh`, `scripts/tiktoken-cache.sh` and the commit (`install.sh` checks `git rev-parse HEAD`).

## Fix
- **Azure and GitLab:** `git init`, `remote add origin`, `sparse-checkout set deploy/ci scripts`, `fetch --depth 1 --filter=blob:none origin <VIZZINI_REF>`, `checkout FETCH_HEAD`. Git fetches the commit and its trees, then only the files the sparse checkout needs. Cone mode also checks out the root files.
- **GitHub:** `actions/checkout` with `sparse-checkout: deploy/ci, scripts`. It already fetches depth 1, and with `sparse-checkout` it adds `blob:none`.
- **Limit:** without `VIZZINI_VERSION`, `install.sh` builds `vizzini-bot` from source and needs the whole commit. The template comments and the CI README say to remove the sparse checkout then.
- No release is needed: the templates change, and `VIZZINI_REF` stays the v0.1.3 commit.

Rejected: **keep the checkout in the tools cache.** It needs a second cache path in every template, and the sparse fetch is already small.

## Acceptance criteria
- [x] `make ci-check` passes. `install.sh` (v0.1.3) runs from the sparse checkout: it installs and verifies all tools, and the tree stays clean. stubllm builds in it (vizzini-lab needs that).
- [x] vizzini-lab's template suites pass live, and the clone time drops.

## Resolution
`54b211a` changes the three templates. vizzini-lab pinned it and ran the template suites live on 2026-11-22, on stubllm: GitHub 17/17, Azure 17/17, GitLab 18/18.

Time from the start of the Azure install step to the first `install.sh` line (the Vizzini fetch), on the self-hosted agent:

| | build 1 | build 2 | build 3 |
|---|---|---|---|
| full clone (run 20261123T002004Z) | 27.9 s | 38.9 s | 11.4 s |
| sparse, blobless, depth 1 (run 20261123T010807Z) | 7.1 s | 3.1 s | 2.1 s |

On a warm agent, the whole install step now takes 2.5 to 3.6 s. On GitHub-hosted runners the checkout was already under 1.5 s before and after. Record: vizzini-lab `results/2026-11-22-ci-sparse-vizzini-fetch.md`.
