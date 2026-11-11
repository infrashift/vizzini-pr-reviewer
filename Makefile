.PHONY: all test vet-cue lint build tiktoken-cache wiki clean

GOBIN    := $(shell go env GOPATH)/bin
GOLANGCI ?= $(GOBIN)/golangci-lint
CUE      ?= cue

# Pure Go everywhere (ADR-0002). The one exception is `test`: Go's race
# detector runtime needs cgo on Linux, so it re-enables cgo for that target only.
export CGO_ENABLED := 0

# tiktoken-go reads BPE encodings from this cache and only falls back to the
# network on a miss. `make tiktoken-cache` seeds it so tests and offline
# microVMs never need network access (ISSUE-001).
TIKTOKEN_CACHE_DIR ?= $(CURDIR)/.cache/tiktoken
export TIKTOKEN_CACHE_DIR

all: lint test build

## test: unit tests with the race detector (seeds the tiktoken cache first)
test: tiktoken-cache
	CGO_ENABLED=1 go test -v -race ./...

## vet-cue: validate the CUE schemas (schema/ arrives in Phase 2)
vet-cue:
	@if [ -d schema ]; then $(CUE) vet ./schema/...; else echo "vet-cue: no schema/ directory yet, skipping"; fi

## lint: golangci-lint (strict config in .golangci.yml) plus go mod tidy check
lint:
	$(GOLANGCI) run
	go mod tidy -diff

## build: static CLI binary
build:
	go build -o bin/pr-review ./cmd/pr-review

## tiktoken-cache: download and checksum-verify the BPE encodings
tiktoken-cache:
	@scripts/tiktoken-cache.sh "$(TIKTOKEN_CACHE_DIR)"

## wiki: validate the OKF wiki
wiki:
	okfw validate --all
	okfw validate --okf

clean:
	rm -rf bin/
