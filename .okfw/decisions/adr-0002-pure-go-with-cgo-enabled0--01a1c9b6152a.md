---
id: "01a1c9b6-152a-7eb5-90f2-5e272253c3f2"
title: "ADR-0002: Pure Go with CGO_ENABLED=0"
type: "ADR"
tags:
  - "adr"
  - "build"
  - "portability"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Vizzini is strictly pure Go and ships as a static CGO_ENABLED=0 binary for UBI9 containers and QEMU microVMs."
---

# ADR-0002: Pure Go with CGO_ENABLED=0

**Status:** Accepted (amended 2026-11-11: race detector exception, see Amendment 1)
**Date:** 2026-11-11
**Source:** `CLAUDE.md`, `PLAN.md`

## Context
Vizzini runs in-process inside the `mrman` TUI and as an isolated CLI inside Red Hat UBI9 containers or QEMU microVMs. Those targets favor minimal images, no libc coupling, and reproducible cross-compilation.

## Decision
- Go 1.23+ with `CGO_ENABLED=0` for every build and test.
- `make build` produces `bin/pr-review` as a static binary, checked with `file bin/pr-review`.
- Every dependency must be pure Go. Git and diff handling uses `go-git` and `go-gitdiff`; code analysis uses the standard library's `go/parser` and `go/ast`.

## Consequences
- cgo libraries are ruled out: tree-sitter bindings, libgit2/git2go, native tokenizers. Multi-language AST analysis stays out of scope until a pure-Go option exists.
- `tiktoken-go` fetches its BPE rank files over the network at first use by default. That breaks in isolated microVMs. Resolved by [ADR-0006: Offline token counting via a pre-seeded TIKTOKEN_CACHE_DIR](/decisions/adr-0006-offline-token-counting-via-a-pre-seeded-tiktoken-cache-dir--01a1c9be0019.md).

## Alternatives considered
- **cgo + tree-sitter for multi-language analysis.** Rejected for now: it breaks static builds and complicates the UBI9 and microVM images.

## Amendment 1 (2026-11-11): race detector needs cgo
Found in [ISSUE-001](/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md): on Linux, `go test -race` fails with "-race requires cgo". CLAUDE.md mandates both `-race` and `CGO_ENABLED=0`.
- The Makefile exports `CGO_ENABLED=0` for every target. `make test` alone overrides it to `CGO_ENABLED=1`, because the race detector's runtime (ThreadSanitizer) needs it.
- This doesn't weaken the pure-Go guarantee. Vizzini's own code and dependencies contain no cgo. `make lint` (golangci-lint, `go vet`) and `make build` run with `CGO_ENABLED=0`, so a cgo-only dependency fails there.
