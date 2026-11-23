---
x-okfw-id: "01a205bd-4377-7e39-8217-aae2525b53c3"
title: "ADR-0027: The CI templates install only release binaries and need no Go"
type: "ADR"
tags:
  - "adr"
  - "ci"
  - "release"
x-okfw-created: "2026-11-22T18:10:19-06:00"
x-okfw-timestamp: "2026-11-22T18:10:19-06:00"
description: "With garmr v0.1.0 released, install.sh downloads every tool from a checksummed release, so the templates drop their Go setup and GitLab uses a small image with git and curl."
---

# ADR-0027: The CI templates install only release binaries and need no Go

**Status:** Accepted
**Date:** 2026-11-22
**Related:** [ISSUE-036](/issues/issue-036-install-sh-downloads-every-tool-on-every-job-also-on-a-self-hosted-agent--01a205a97301.md), [ADR-0026](/decisions/adr-0026-install-sh-keeps-verified-tools-in-a-cache-under-their-pins--01a205a972f2.md) (tools cache), [ADR-0025](/decisions/adr-0025-release-archives-for-linux-amd64-and-arm64--01a1ffcd31a4.md) (release archives), [ISSUE-035](/issues/issue-035-install-sh-does-not-retry-a-reset-download--01a2007408e3.md)

## Context
`install.sh` built garmr with `go install` at a pinned commit, because garmr had no release. That was the only reason the CI jobs needed Go: the templates set it up (`GoTool@0`, `actions/setup-go`, a `golang` image on GitLab), and `go install` fetched the go1.26.11 toolchain that garmr requires. That download failed a job once ([ISSUE-035](/issues/issue-035-install-sh-does-not-retry-a-reset-download--01a2007408e3.md)). garmr v0.1.0 has release archives for Linux and macOS on amd64 and arm64, with `SHA256SUMS` and a provenance attestation. Its code is the same as the commit the templates pinned (`bf96753`); the release added only release tooling.

## Decision
- `install.sh` downloads garmr like mrman: the `GARMR_VERSION` release archive, checked against its `SHA256SUMS`, then `garmr version` must report the pin. `GARMR_REF` is no longer used; the script says so if it is set.
- The script gets the OS and architecture from `uname`, not from `go env`. Go is needed only to build `vizzini-bot` from source, when `VIZZINI_VERSION` is unset.
- The templates pin `GARMR_VERSION: v0.1.0` and drop their Go setup. The GitLab job uses `buildpack-deps:bookworm-scm` (bash, git, curl) instead of `golang`.

## Consequences
- A job installs three release binaries and the tokenizer files. Without a cache it needs only curl, tar, sha256sum and git.
- A template copied before this change keeps working with its old `VIZZINI_REF`. When it moves to the new release, it must rename `GARMR_REF` to `GARMR_VERSION`; otherwise the default, v0.1.0, applies.
- vizzini-lab's template tests add Go themselves for the stubllm step they inject.

