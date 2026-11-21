---
id: "01a1fff0-f982-753d-bc47-4044edb958aa"
title: "ISSUE-034: Use the v0.1.0 release downstream"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "release"
  - "ci"
  - "mrman"
  - "in-progress"
created: "2026-11-21T15:09:05-06:00"
timestamp: "2026-11-21T15:09:05-06:00"
description: "mrman pins Vizzini v0.1.0 instead of a pseudo-version, and install.sh downloads the released vizzini-bot, checked against SHA256SUMS, instead of building it."
---

# ISSUE-034: Use the v0.1.0 release downstream


**Status:** in-progress · **Raised:** 2026-11-21 · **Related:** [ISSUE-033](/issues/issue-033-release-v0-1-0--01a1ffcd31b4.md), [ADR-0025](/decisions/adr-0025-release-archives-for-linux-amd64-and-arm64--01a1ffcd31a4.md)

## Problem
ISSUE-033 released v0.1.0, but nothing used it:
- mrman required Vizzini at the pseudo-version `v0.0.0-20261112115612-0187f14d29af`.
- `deploy/ci/install.sh` built `vizzini-bot` from the pinned checkout on every CI run.

## Fix
- **mrman:** `go.mod` requires `github.com/infrashift/vizzini-pr-reviewer v0.1.0`. mrman imports only `pkg/risk` and `pkg/marker`. Neither changed between `0187f14` and `v0.1.0`, so mrman's behaviour does not change.
- **install.sh:** with `VIZZINI_VERSION` set, the script downloads that release's archive and checks it against the release's `SHA256SUMS`. It shares one `fetch_release` function with the mrman download. It then stops unless `vizzini-bot --version` reports the commit of the checkout, so the binary and the checked-out scripts always match. Without `VIZZINI_VERSION`, it builds from the checkout as before. A non-Linux host with `VIZZINI_VERSION` set stops with a message, because releases are Linux only.
- **Templates:** `install.sh` at `v0.1.0` does not have the download, so the templates need a release that does. That release is `v0.1.1`. The templates then pin `VIZZINI_REF` to its commit and set `VIZZINI_VERSION: v0.1.1`.

## Acceptance criteria
- [x] mrman requires Vizzini v0.1.0; mrman `make check` passes (87.5% coverage, 0 lint issues).
- [x] `install.sh` downloads and verifies the release; shellcheck passes. Checked locally: a v0.1.0 checkout with `VIZZINI_VERSION=v0.1.0` installs; a different checkout stops with the commit mismatch; no `VIZZINI_VERSION` builds from source.
- [x] The CI README and the bot guide describe both modes.
- [ ] mrman change merged.
- [ ] `v0.1.1` released; the three templates pin `VIZZINI_REF` and `VIZZINI_VERSION` to it; `make ci-check` passes.
