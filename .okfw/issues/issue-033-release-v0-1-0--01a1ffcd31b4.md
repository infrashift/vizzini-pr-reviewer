---
id: "01a1ffcd-31b4-79d9-9401-64ad0fd71b6d"
title: "ISSUE-033: Release v0.1.0"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "release"
  - "closed"
created: "2026-11-21T14:30:00-06:00"
timestamp: "2026-11-21T14:30:00-06:00"
description: "Ship Vizzini's first tagged release, v0.1.0, as Linux amd64 and arm64 archives built and published by a tag-driven workflow."
---

# ISSUE-033: Release v0.1.0


**Status:** closed (released as v0.1.0) · **Raised:** 2026-11-21 · **Related:** [ADR-0025](/decisions/adr-0025-release-archives-for-linux-amd64-and-arm64--01a1ffcd31a4.md)

## Problem
Vizzini had no tagged release. Users built it from source. The binaries had no version flag, so a report or a CI log could not name the build that made it.

## Scope
A first release, `v0.1.0`, for Linux on amd64 and arm64.

## Acceptance criteria
- [x] Each binary prints its version, commit and build date with `--version` (`internal/buildinfo`, with tests).
- [x] `make dist` and `make release` build the archives and `SHA256SUMS` for linux/amd64 and linux/arm64.
- [x] `.github/workflows/release.yml` builds, smoke-tests on both architectures and publishes the release with `SHA256SUMS` and a provenance attestation. `actionlint` passes.
- [x] The docs say how to install and verify a release (Getting Started, Make Targets).
- [x] Tag `v0.1.0` pushed; the release workflow passes; the release has both archives, `SHA256SUMS` and the attestation.
- [x] A downloaded archive passes `sha256sum -c` and `gh attestation verify`, and its binaries report `v0.1.0`.

## Resolution
Released [v0.1.0](https://github.com/infrashift/vizzini-pr-reviewer/releases/tag/v0.1.0) on 2026-11-21 from commit `5675b7e`. Release workflow run 44949115102 passed: both builds, the smoke tests on `ubuntu-latest` and `ubuntu-24.04-arm`, and the publish.

The release holds `vizzini_0.1.0_linux_amd64.tar.gz`, `vizzini_0.1.0_linux_arm64.tar.gz` and `SHA256SUMS`. Checked from a clean download:
- `sha256sum -c SHA256SUMS` passes for both archives.
- Each archive has one SLSA provenance v1 attestation, built from `refs/tags/v0.1.0`. The local `gh` 2.45 has no `attestation` command, so the check used the attestations API by digest.
- The amd64 binaries report `v0.1.0` and commit `5675b7e`. The smoke job ran the arm64 binaries on an arm64 runner.
- The Go module proxy serves `github.com/infrashift/vizzini-pr-reviewer@v0.1.0`.

**Follow-ups ([ISSUE-034](/issues/issue-034-use-the-v0-1-0-release-downstream--01a1fff0f982.md)):** move mrman's pin from the pseudo-version to `v0.1.0`. `deploy/ci/install.sh` could download a verified `vizzini-bot` instead of building it from a checkout.
