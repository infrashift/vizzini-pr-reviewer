---
title: Getting Started
description: Build Vizzini from source and review your first diff.
---

Vizzini has no tagged release yet, so build it from source. The engine, the
`pkg/reviewer` Go API and the `pr-review` CLI are complete and tested.

## Requirements

| Tool | Version | Used for |
|---|---|---|
| Go | 1.25+ | Building and testing (`cuelang.org/go` sets the floor) |
| `cue` CLI | v0.17.1 | `make vet-cue` |
| `golangci-lint` | v2 | `make lint` |
| `curl`, `sha256sum` | any | Seeding the tokenizer cache once |
| A C compiler | any | Only for `make test`, because Go's race detector needs cgo |
| An OpenAI-compatible endpoint | | A LiteLLM proxy in front of the model you review with |

## Build and test

```sh
git clone https://github.com/infrashift/vizzini-pr-reviewer
cd vizzini-pr-reviewer
make test      # seeds .cache/tiktoken on first run, then runs go test -race
make vet-cue   # cue vet ./schema/...
make lint      # golangci-lint + go mod tidy check
make build     # static bin/pr-review
```

The first `make test` downloads the two tokenizer encodings (about 5.3 MB)
and checks them against pinned SHA-256 hashes. After that, everything runs
offline. The tests actively block network access. See
[Running Offline](../guides/offline/).

## Your first review

```sh
export VIZZINI_LITELLM_URL=http://litellm:4000/v1
export VIZZINI_MODEL=<model name in your LiteLLM config>
export VIZZINI_API_KEY=<proxy key>
export TIKTOKEN_CACHE_DIR=$PWD/.cache/tiktoken

git diff main...HEAD | ./bin/pr-review --mode review
```

You get Markdown: a summary, a risk score, and findings sorted most severe
first, each pinned to `file:line`. Add `--format json` for machines, or try
`--mode describe` for a PR description and `--mode improve` for concrete code
suggestions.

Next: the [CLI Reference](../reference/cli/), [Embedding in Go](../guides/embedding/),
or [How a Review Works](../guides/how-it-works/).
