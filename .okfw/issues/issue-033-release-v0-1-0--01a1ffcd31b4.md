---
id: "01a1ffcd-31b4-79d9-9401-64ad0fd71b6d"
title: "ISSUE-033: Release v0.1.0"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "release"
  - "in-progress"
created: "2026-11-21T14:30:00-06:00"
timestamp: "2026-11-21T14:30:00-06:00"
description: "Ship Vizzini's first tagged release, v0.1.0, as Linux amd64 and arm64 archives built and published by a tag-driven workflow."
---

# ISSUE-033: Release v0.1.0


**Status:** in-progress · **Raised:** 2026-11-21 · **Related:** [ADR-0025](/decisions/adr-0025-release-archives-for-linux-amd64-and-arm64--01a1ffcd31a4.md)

## Problem
Vizzini had no tagged release. Users built it from source. The binaries had no version flag, so a report or a CI log could not name the build that made it.

## Scope
A first release, `v0.1.0`, for Linux on amd64 and arm64.

## Acceptance criteria
- [x] Each binary prints its version, commit and build date with `--version` (`internal/buildinfo`, with tests).
- [x] `make dist` and `make release` build the archives and `SHA256SUMS` for linux/amd64 and linux/arm64.
- [x] `.github/workflows/release.yml` builds, smoke-tests on both architectures and publishes the release with `SHA256SUMS` and a provenance attestation. `actionlint` passes.
- [x] The docs say how to install and verify a release (Getting Started, Make Targets).
- [ ] Tag `v0.1.0` pushed; the release workflow passes; the release has both archives, `SHA256SUMS` and the attestation.
- [ ] A downloaded archive passes `sha256sum -c` and `gh attestation verify`, and its binaries report `v0.1.0`.
