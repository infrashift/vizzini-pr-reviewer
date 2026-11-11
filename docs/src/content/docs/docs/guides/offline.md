---
title: Running Offline
description: Seed the tokenizer cache so Vizzini never needs the network, in tests, containers or microVMs.
---

Vizzini counts tokens with `tiktoken-go`. On first use, that library
downloads its BPE encoding files from `openaipublic.blob.core.windows.net`
**unless** it finds them in `TIKTOKEN_CACHE_DIR`. Isolated QEMU microVMs and
air-gapped containers have no network, so Vizzini always pre-seeds that cache.

## Seed the cache

```sh
make tiktoken-cache                         # into ./.cache/tiktoken
scripts/tiktoken-cache.sh /opt/vizzini/tiktoken   # or anywhere else
```

The script stores each encoding under the file name tiktoken-go looks for
(the SHA-1 of its download URL). It checks each file against a pinned
SHA-256 hash, the same values OpenAI's own `tiktoken` uses:

| Encoding | SHA-256 |
|---|---|
| `cl100k_base` | `223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7` |
| `o200k_base` | `446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d` |

Re-running is cheap: files that are already present and correct are skipped.

## Use it at runtime

Point `TIKTOKEN_CACHE_DIR` at the seeded directory. In a container image:

```dockerfile
COPY .cache/tiktoken /opt/vizzini/tiktoken
ENV TIKTOKEN_CACHE_DIR=/opt/vizzini/tiktoken
```

If the cache is missing, the chunker fails with an error that points at
`make tiktoken-cache`. It doesn't hang on a download that can never succeed.

## How the tests stay honest

The chunker tests route all HTTP(S) traffic to a dead proxy before anything
runs. A cache miss therefore fails loudly instead of quietly downloading the
encodings, and `TestNew_CacheMissFailsOffline` checks exactly that.
