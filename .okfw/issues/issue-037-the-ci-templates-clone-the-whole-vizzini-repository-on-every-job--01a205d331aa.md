---
id: "01a205d3-31aa-7958-a98e-71bf88df0f3b"
title: "ISSUE-037: The CI templates clone the whole Vizzini repository on every job"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "ci"
  - "performance"
  - "open"
created: "2026-11-22T18:34:16-06:00"
timestamp: "2026-11-22T18:34:16-06:00"
description: "On a warm self-hosted agent the tools install in under a second, but the job still clones Vizzini's full history to run install.sh, 11 to 39 s in vizzini-lab; a shallow fetch of the pinned commit would do."
---

# ISSUE-037: The CI templates clone the whole Vizzini repository on every job

**Status:** open · **Raised:** 2026-11-22 · **Related:** [ISSUE-036](/issues/issue-036-install-sh-downloads-every-tool-on-every-job-also-on-a-self-hosted-agent--01a205a97301.md), [ADR-0026](/decisions/adr-0026-install-sh-keeps-verified-tools-in-a-cache-under-their-pins--01a205a972f2.md), [2026-11-22 daily note](/daily/2026/11/2026-11-22-sunday--01a2008d7400.md)

## Problem
Each template clones `infrashift/vizzini-pr-reviewer` with its full history (`git clone --no-checkout`, or `actions/checkout` on GitHub) only to run `deploy/ci/install.sh` and `scripts/tiktoken-cache.sh` from the pinned commit. In vizzini-lab's Azure suite on 2026-11-22 (run 20261123T002004Z), builds 2 and 3 installed every tool from the cache in under 0.3 s, but the clone before it took 39 s and 11 s. The clone is now most of the install step on a warm agent, and it needs the network.

## Options
- **Shallow fetch:** `git init`, `git fetch --depth 1 origin <VIZZINI_REF>`, `git checkout FETCH_HEAD`. GitHub serves a commit by its SHA. `install.sh` checks the commit with `git rev-parse HEAD`, which works in a shallow clone. On GitHub, `actions/checkout` already fetches depth 1 by default.
- **Keep the checkout in the tools cache** under `VIZZINI_REF`, like a tool entry.

## Acceptance criteria
- [ ] The Azure and GitLab templates fetch only the pinned commit.
- [ ] vizzini-lab's template suites pass live, and the Azure clone time drops.

