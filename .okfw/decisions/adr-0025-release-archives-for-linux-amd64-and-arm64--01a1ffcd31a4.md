---
id: "01a1ffcd-31a4-7808-87b6-39c922aebb97"
title: "ADR-0025: Release archives for Linux amd64 and arm64"
type: "ADR"
tags:
  - "adr"
  - "release"
  - "build"
created: "2026-11-21T14:30:00-06:00"
timestamp: "2026-11-21T14:30:00-06:00"
description: "A v* tag builds one archive per Linux architecture (amd64, arm64) holding all three binaries, stamped with the version, checked by SHA256SUMS and a SLSA provenance attestation."
---

# ADR-0025: Release archives for Linux amd64 and arm64


**Status:** Accepted
**Date:** 2026-11-21
**Related:** [ISSUE-033](/issues/issue-033-release-v0-1-0--01a1ffcd31b4.md), [ADR-0002](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md) (pure Go, so every target cross-compiles)

## Context
Vizzini had no tagged release. Users built it from source, and mrman pinned a pseudo-version. The CI templates build `vizzini-bot` from a checkout. The targets are Linux CI runners, UBI9 containers and QEMU microVMs, on amd64 and arm64.

## Decision
- **Platforms:** linux/amd64 and linux/arm64 only. No macOS or Windows archives.
- **One archive per platform:** `vizzini_<version>_linux_<arch>.tar.gz`. It holds `pr-review`, `vizzini-bot`, `vizzini-webhook`, `README.md` and `tiktoken-cache.sh`. The archive layout is the same as mrman's and whac's, so `install.sh`-style scripts work the same way.
- **Version stamping:** `internal/buildinfo` holds `Version`, `Commit` and `Date`. `make build`, `make dist` and the workflow set them with `-ldflags -X`. Each binary prints them with `--version`. A `go install ...@vX.Y.Z` build has no ldflags, so the module version and VCS data from the Go toolchain fill the gaps.
- **Workflow:** `.github/workflows/release.yml` runs on a `v*` tag. It builds each archive, smoke-tests it on a runner of its own architecture (`ubuntu-latest`, `ubuntu-24.04-arm`), then publishes the GitHub release with `SHA256SUMS` and a SLSA build provenance attestation.
- **Smoke test:** each binary must report the tag with `--version`, and `pr-review --mode risk` must score a small diff. Risk mode needs no model and no tokenizer cache.

## Consequences
- The same tag versions the `pkg/reviewer` Go module. mrman can pin `v0.1.0` instead of a pseudo-version.
- The CI templates can download a verified `vizzini-bot` in a later change, instead of building it from a checkout.
- A macOS or Windows user builds from source.
