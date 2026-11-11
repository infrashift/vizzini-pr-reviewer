---
title: Getting Started
description: Build Vizzini from source and run its offline test suite.
---

Vizzini is in early development. The engine core (diff parsing, chunking, the
CUE validator and the LiteLLM client) is done and tested. The `pr-review` CLI
and the `pkg/reviewer` Go API are the next milestone, and this page will cover
them once they land.

## Requirements

| Tool | Version | Used for |
|---|---|---|
| Go | 1.25+ | Building and testing (`cuelang.org/go` sets the floor) |
| `cue` CLI | v0.17.1 | `make vet-cue` |
| `golangci-lint` | v2 | `make lint` |
| `curl`, `sha256sum` | any | Seeding the tokenizer cache once |
| A C compiler | any | Only for `make test`, because Go's race detector needs cgo |

## Build and test

```sh
git clone https://github.com/infrashift/vizzini-pr-reviewer
cd vizzini-pr-reviewer
make test      # seeds .cache/tiktoken on first run, then runs go test -race
make vet-cue   # cue vet ./schema/...
make lint      # golangci-lint + go mod tidy check
```

The first `make test` downloads the two tokenizer encodings (about 5.3 MB)
and checks them against pinned SHA-256 hashes. After that, everything runs
offline. The tests actively block network access. See
[Running Offline](../guides/offline/).

## What you can use today

The engine lives in internal packages that the upcoming `pkg/reviewer` API
wraps:

| Package | What it does |
|---|---|
| `internal/diff` | Parses a unified diff, drops generated paths, labels each line with old/new line numbers |
| `internal/chunker` | Packs files into chunks within a token budget, splitting oversized files only between hunks |
| `internal/schema` | Validates raw LLM JSON against `schema/review.cue` and decodes it into Go structs |
| `internal/llm` | Renders prompt templates and calls a LiteLLM / OpenAI-compatible endpoint with retries and schema repair |

Next: [How a Review Works](../guides/how-it-works/).
