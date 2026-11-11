---
id: "01a1c9b6-154f-7a07-9b0d-3db790515b1e"
title: "ISSUE-001: Diff parsing and token-aware chunking"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-1"
  - "diff"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Parse unified diffs, filter generated files, annotate line numbers, and pack hunks into token-bounded chunks."
---

# ISSUE-001: Diff parsing and token-aware chunking

**Status:** open · **Phase:** 1 · **PLAN.md:** Phase 1: MVP Core — Diff Parsing & Token-Aware Chunking

## Description
Parse raw unified diffs, strip noisy generated artifacts, and pack file hunks into token-bounded batches without splitting individual hunks mid-line.

## Acceptance Criteria
- [ ] Initialize `go.mod` and the `Makefile` (`test`, `vet-cue`, `lint`, `build`)
- [ ] `internal/diff/parser.go`: go-gitdiff parsing into file/text fragments
- [ ] Exclude generated paths by configurable globs (`*.pb.go`, `*_gen.go`, `go.sum`, `vendor/**`, `*.lock`)
- [ ] Annotate each hunk line with explicit old/new line numbers
- [ ] `internal/chunker/chunker.go`: tiktoken-go packing below `MaxChunkTokens` (default 6000)
- [ ] Split oversized single files at `*gitdiff.TextFragment` boundaries
- [ ] `make test` covers binary diffs, ignored lockfiles, multi-file packing, oversized-file splitting

## Notes
- **Open question:** `tiktoken-go` downloads BPE rank files at first use by default, which fails in offline microVMs (see ADR-0002). Options: set `TIKTOKEN_CACHE_DIR` to a pre-seeded directory, or use an offline loader (`pkoukk/tiktoken-go-loader`, which isn't on the pinned list, so it needs an ADR). Tests must not need the network.
- A single hunk larger than `MaxChunkTokens` can't be split without breaking the no-mid-hunk rule. Decide whether to emit it as its own oversized chunk or truncate it, and record the choice.

## Related
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [Vizzini architecture overview](/architecture/vizzini-architecture-overview--01a1c9b61549.md)
