---
title: Getting Started
description: Install a Vizzini release or build from source, then review your first diff.
---

Install a release archive, or build Vizzini from source. Releases are for
Linux on amd64 and arm64.

## Install a release

Each release has one archive per architecture. An archive holds `pr-review`,
`vizzini-bot`, `vizzini-webhook`, `README.md` and `tiktoken-cache.sh`. Check
the archive against the release's `SHA256SUMS` before you extract it.

```sh
VERSION=v0.1.0
ARCH=amd64   # or arm64
name=vizzini_${VERSION#v}_linux_${ARCH}
url=https://github.com/infrashift/vizzini-pr-reviewer/releases/download/$VERSION
curl -fsSLO "$url/$name.tar.gz"
curl -fsSLO "$url/SHA256SUMS"
grep "  $name.tar.gz\$" SHA256SUMS | sha256sum -c -
tar -xzf "$name.tar.gz"
install -m 0755 "$name"/pr-review "$name"/vizzini-bot "$name"/vizzini-webhook ~/.local/bin/
pr-review --version
```

Each archive also has a SLSA build provenance attestation. To verify it:
`gh attestation verify "$name.tar.gz" --repo infrashift/vizzini-pr-reviewer`.

Seed the tokenizer cache once, so that reviews run offline:
`"$name"/tiktoken-cache.sh ~/.cache/vizzini/tiktoken`, then set
`TIKTOKEN_CACHE_DIR` to that directory.

The `pkg/reviewer` Go API uses the same tag:
`go get github.com/infrashift/vizzini-pr-reviewer@v0.1.0`.

## Build from source

The engine, the `pkg/reviewer` Go API and the `pr-review` CLI are complete
and tested.

### Requirements

| Tool | Version | Used for |
|---|---|---|
| Go | 1.25+ | Building and testing (`cuelang.org/go` sets the floor) |
| `cue` CLI | v0.17.1 | `make vet-cue` |
| `golangci-lint` | v2 | `make lint` |
| `curl`, `sha256sum` | any | Seeding the tokenizer cache once |
| A C compiler | any | Only for `make test`, because Go's race detector needs cgo |
| An OpenAI-compatible endpoint | | A LiteLLM proxy in front of the model you review with |

### Build and test

```sh
git clone https://github.com/infrashift/vizzini-pr-reviewer
cd vizzini-pr-reviewer
make test      # seeds .cache/tiktoken on first run, then runs go test -race
make vet-cue   # cue vet ./schema/...
make lint      # golangci-lint + go mod tidy check
make build     # static binaries in bin/
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
