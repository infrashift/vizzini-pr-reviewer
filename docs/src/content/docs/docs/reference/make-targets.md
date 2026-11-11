---
title: Make Targets
description: The GNU Make targets that build, test and verify Vizzini.
---

Every change must pass `make test && make vet-cue && make lint` before it is
committed.

| Target | Runs | Notes |
|---|---|---|
| `make test` | `go test -v -race ./...` | Seeds the tokenizer cache first. Re-enables cgo for this target only, because the race detector needs it |
| `make vet-cue` | `cue vet ./schema/...` | Validates the canonical CUE schemas |
| `make lint` | `golangci-lint run`, `go mod tidy -diff` | Strict v2 config in `.golangci.yml` |
| `make build` | `go build -o bin/pr-review ./cmd/pr-review` | `CGO_ENABLED=0` static binary |
| `make tiktoken-cache` | `scripts/tiktoken-cache.sh` | Downloads and checksum-verifies the BPE encodings |
| `make e2e-lint` | `scripts/e2e-golint.sh` | Real golangci-lint + git + `pr-review` end-to-end proof; stand-in model unless `VIZZINI_LITELLM_URL` is set |
| `make e2e-garmr` | `scripts/e2e-garmr.sh` | `pr-review` reports evaluated by a real `garmr-server` with `policies/garmr`; skips without garmr |
| `make wiki` | `okfw validate --all && okfw validate --okf` | Checks the project's OKF knowledge base in `.okfw/` |
| `make docs-dev` | `bun run dev` in `docs/` | This site, with live reload |
| `make docs-build` | `bun run build` in `docs/` | Builds the site into `docs/dist/` |

`CGO_ENABLED=0` is exported for every target. A dependency that needed cgo
would therefore fail `lint` and `build` even though `test` enables cgo for the
race detector.
