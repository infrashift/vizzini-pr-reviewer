---
id: "01a1c9b6-154f-7a07-9b0d-3db790515b1e"
title: "ISSUE-001: Diff parsing and token-aware chunking"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-1"
  - "diff"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:34:16-06:00"
description: "Parse unified diffs, filter generated files, annotate line numbers, and pack hunks into token-bounded chunks."
---

# ISSUE-001: Diff parsing and token-aware chunking

**Status:** closed · **Phase:** 1 · **PLAN.md:** Phase 1: MVP Core — Diff Parsing & Token-Aware Chunking

## Description
Parse raw unified diffs, strip noisy generated artifacts, and pack file hunks into token-bounded batches without splitting individual hunks mid-line.

## Acceptance Criteria
- [x] Initialize `go.mod` and the `Makefile` (`test`, `vet-cue`, `lint`, `build`)
- [x] `internal/diff/parser.go`: go-gitdiff parsing into file/text fragments
- [x] Exclude generated paths by configurable globs (`*.pb.go`, `*_gen.go`, `go.sum`, `vendor/**`, `*.lock`)
- [x] Annotate each hunk line with explicit old/new line numbers
- [x] `internal/chunker/chunker.go`: tiktoken-go packing below `MaxChunkTokens` (default 6000)
- [x] Split oversized single files at `*gitdiff.TextFragment` boundaries
- [x] `make test` covers binary diffs, ignored lockfiles, multi-file packing, oversized-file splitting

## Notes
- **Resolved by ADR-0006:** `tiktoken-go` downloads BPE rank files at first use by default, which fails in offline microVMs (see ADR-0002). Options: set `TIKTOKEN_CACHE_DIR` to a pre-seeded directory, or use an offline loader (`pkoukk/tiktoken-go-loader`, which isn't on the pinned list, so it needs an ADR). Tests must not need the network.
- **Resolved (see Resolution):** A single hunk larger than `MaxChunkTokens` can't be split without breaking the no-mid-hunk rule. Decide whether to emit it as its own oversized chunk or truncate it, and record the choice.

## Resolution
Done 2026-11-11.
- **Build:** `go.mod` (`go 1.23`, requires go-gitdiff v0.9.0 and tiktoken-go v0.1.8). The `Makefile` has `test`, `vet-cue`, `lint`, `build`, `tiktoken-cache` and `wiki`. `.golangci.yml` is the strict v2 config adapted from tenet-cac. `make lint` also runs `go mod tidy -diff`. `make vet-cue` skips until `schema/` exists (Phase 2).
- **`internal/diff`:** `Parse(r, Options) (*Diff, error)` returns `Files` and `Excluded`. `Options.Exclude` set to nil means `DefaultExcludes`; a non-nil empty slice excludes nothing. Glob semantics: a pattern without `/` matches the base name at any depth; a pattern with `/` is anchored at the repo root, and `**` matches zero or more segments. No doublestar dependency. Each `Line` has `OldLine`/`NewLine` (0 when absent). `File.Header()` gives a one-line description (added/deleted/renamed/copied, mode change, binary). `Hunk.Render()` prints a fixed-width `old new op text` gutter. `File.Source` keeps the `*gitdiff.File` for Milestones 4.2/4.3.
- **`internal/chunker`:** `New(Options)` loads the encoding once; the default is `cl100k_base`, with `o200k_base` also seeded. `Chunk(files)` packs files greedily in order. A file that fits is never split; a file that doesn't is split at hunk boundaries, and later parts get a `[continued]` header.
- **Decision: oversized single hunk.** A hunk that alone exceeds the budget becomes its own chunk with `Chunk.Oversized = true`. It is never truncated or split mid-hunk, and the caller (Phase 3) decides whether to send, truncate or skip it.
- **Exact budgets.** Every rendered piece starts with a non-whitespace marker (`###` or `@@`) and ends in a newline, so BPE pre-tokenization doesn't merge across pieces and per-piece counts add up exactly. Tests check `CountTokens(Render()) == Tokens` for every chunk and part.
- **Tests** (`make test`, race on): binary file, rename plus mode change, added and deleted files, excluded lockfile/`go.sum`/vendor/`.pb.go`/`_gen.go`, custom and invalid globs, exact line numbers and render output, malformed hunk, multi-file greedy packing (12 files into exactly 4 chunks), oversized-file splitting with header and continuation checks, a single hunk over budget, concurrent `CountTokens`, and a cache miss failing offline.
- **Follow-ups:** `-race` needs cgo, recorded as an amendment to ADR-0002. The tiktoken cache decision is [ADR-0006: Offline token counting via a pre-seeded TIKTOKEN_CACHE_DIR](/decisions/adr-0006-offline-token-counting-via-a-pre-seeded-tiktoken-cache-dir--01a1c9be0019.md).

## Related
- [ADR-0006: Offline token counting via a pre-seeded TIKTOKEN_CACHE_DIR](/decisions/adr-0006-offline-token-counting-via-a-pre-seeded-tiktoken-cache-dir--01a1c9be0019.md)
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [Vizzini architecture overview](/architecture/vizzini-architecture-overview--01a1c9b61549.md)
