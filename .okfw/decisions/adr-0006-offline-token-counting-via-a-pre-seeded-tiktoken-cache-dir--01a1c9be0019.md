---
id: "01a1c9be-0019-78c2-b7cf-934660a024d3"
title: "ADR-0006: Offline token counting via a pre-seeded TIKTOKEN_CACHE_DIR"
type: "ADR"
tags:
  - "adr"
  - "tokenizer"
  - "offline"
  - "build"
created: "2026-11-11T02:33:54-06:00"
timestamp: "2026-11-11T02:33:54-06:00"
description: "tiktoken encodings are checksum-pinned and pre-seeded into TIKTOKEN_CACHE_DIR so token counting never needs the network."
---

# ADR-0006: Offline token counting via a pre-seeded TIKTOKEN_CACHE_DIR

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-001](/issues/issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md) open question; decided by the project owner.

## Context
`tiktoken-go` (v0.1.8) loads BPE rank files through `readFileCached`. It looks for `<TIKTOKEN_CACHE_DIR>/<sha1(url)>` and, on a miss, downloads from `openaipublic.blob.core.windows.net` with a plain `http.Get`. Isolated QEMU microVMs and air-gapped UBI9 containers have no network, and unit tests shouldn't depend on it. See [ADR-0002](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md).

## Decision
- Pre-seed the cache instead of adding a loader dependency.
- `scripts/tiktoken-cache.sh <dir>` (run by `make tiktoken-cache`, a prerequisite of `make test`) downloads `cl100k_base` and `o200k_base` into the sha1-of-URL filenames tiktoken-go expects. It verifies them against SHA-256 pins (the same values as OpenAI's `tiktoken` `openai_public.py`):
  - `cl100k_base`: `223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7`
  - `o200k_base`: `446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d`
- The Makefile exports `TIKTOKEN_CACHE_DIR=$(CURDIR)/.cache/tiktoken` (gitignored).
- Container and microVM images copy a seeded directory in and set `TIKTOKEN_CACHE_DIR`.
- `chunker.New` wraps encoding-load errors with a hint to run `make tiktoken-cache`.
- The chunker tests' `TestMain` routes HTTP(S) through a dead proxy. A cache miss then fails loudly instead of silently downloading (`TestNew_CacheMissFailsOffline`).

## Consequences
- No new dependency; the pinned list in CLAUDE.md is unchanged.
- The first `make test` on a fresh checkout needs network once (about 5.3 MB). After that, everything runs offline.
- If tiktoken-go changes its cache-key scheme, the seeded files stop matching. The offline test catches that.
- tiktoken-go also keeps a process-wide encoding cache, so only the first `GetEncoding` per encoding touches disk.

## Alternatives considered
- **`pkoukk/tiktoken-go-loader`** (encodings embedded in the binary). Rejected: it's a dependency outside the pinned list and adds about 5 MB to every binary.
- **Committing the encoding files to git.** Rejected: large binary blobs in history. The checksum-pinned fetch gives the same integrity guarantee.
- **Approximating token counts (e.g. bytes/4).** Rejected: PLAN.md requires tiktoken-accurate budgets.
