.PHONY: all test vet-cue lint build dist release ci-check tiktoken-cache wiki docs-dev docs-build e2e-lint e2e-lint-python e2e-lint-csharp e2e-lint-js e2e-garmr clean

GOBIN    := $(shell go env GOPATH)/bin
GOLANGCI ?= $(GOBIN)/golangci-lint
CUE      ?= cue

# Version stamping (internal/buildinfo). A release build sets VERSION from the tag.
PKG     := github.com/infrashift/vizzini-pr-reviewer/internal/buildinfo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)
BINARIES := pr-review vizzini-bot vizzini-webhook

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

## build: static CLI binaries with version information
build:
	@for b in $(BINARIES); do go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$$b ./cmd/$$b || exit 1; done

# Release targets: Linux on amd64 and arm64 (ADR-0025). Pure Go, so both
# cross-compile from any host.
PLATFORMS  ?= linux/amd64 linux/arm64
RELVERSION := $(VERSION:v%=%)

## dist: build one release archive in dist/ for GOOS/GOARCH (default: the host)
dist:
	@goos=$${GOOS:-$$(go env GOOS)}; goarch=$${GOARCH:-$$(go env GOARCH)}; \
	name=vizzini_$(RELVERSION)_$${goos}_$${goarch}; \
	rm -rf dist/stage/$$name && mkdir -p dist/stage/$$name && \
	for b in $(BINARIES); do \
		GOOS=$$goos GOARCH=$$goarch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/stage/$$name/$$b ./cmd/$$b || exit 1; \
	done && \
	cp README.md scripts/tiktoken-cache.sh dist/stage/$$name/ && \
	tar -C dist/stage --owner=0 --group=0 --numeric-owner -czf dist/$$name.tar.gz $$name && \
	echo "dist/$$name.tar.gz"

## release: build every platform archive and dist/SHA256SUMS for VERSION
release:
	@for p in $(PLATFORMS); do $(MAKE) --no-print-directory dist GOOS=$${p%/*} GOARCH=$${p#*/} || exit 1; done
	cd dist && sha256sum vizzini_$(RELVERSION)_*.tar.gz > SHA256SUMS && cat SHA256SUMS

## ci-check: lint the vizzini-bot CI templates and the webhook kit (needs shellcheck, actionlint, cue)
ci-check:
	shellcheck deploy/ci/install.sh deploy/webhook/ado-subscription.sh
	$(GOBIN)/actionlint deploy/ci/github/vizzini-bot.yml .github/workflows/release.yml
	cue export --out json deploy/ci/azure-pipelines/vizzini-bot.yml >/dev/null
	cue export --out json deploy/ci/gitlab/vizzini-bot.gitlab-ci.yml | \
		python3 -I -c 'import json,sys; print("#!/usr/bin/env bash"); print("\n".join(json.load(sys.stdin)["vizzini-bot"]["script"]))' | \
		shellcheck -

## tiktoken-cache: download and checksum-verify the BPE encodings
tiktoken-cache:
	@scripts/tiktoken-cache.sh "$(TIKTOKEN_CACHE_DIR)"

## wiki: validate the OKF wiki
wiki:
	okfw validate --all
	okfw validate --okf

## e2e-lint: real golangci-lint + git + pr-review end-to-end proof (stub model
## unless VIZZINI_LITELLM_URL/VIZZINI_MODEL point at a real proxy)
e2e-lint: build tiktoken-cache
	scripts/e2e-golint.sh

## e2e-lint-python: the same proof for Python, via real ruff SARIF (uvx ruff
## if ruff is not installed; skips without either)
e2e-lint-python: build tiktoken-cache
	scripts/e2e-pylint.sh

## e2e-lint-csharp: the same proof for C#, via real Roslyn SARIF from
## dotnet build in the .NET SDK container (skips without podman)
e2e-lint-csharp: build tiktoken-cache
	scripts/e2e-cslint.sh

## e2e-lint-js: the same proof for TypeScript/JavaScript, via real ESLint +
## typescript-eslint SARIF from the node container (skips without podman)
e2e-lint-js: build tiktoken-cache
	scripts/e2e-jslint.sh

## e2e-garmr: pr-review reports evaluated in-process by garmr eval --policy-dir
## policies/garmr, no server (skips unless garmr is on PATH or GARMR_BIN_DIR is set)
e2e-garmr: build tiktoken-cache
	scripts/e2e-garmr.sh

## docs-dev: run the Astro Starlight site locally (bun)
docs-dev:
	cd docs && bun install && bun --bun run dev

## docs-build: build the site into docs/dist (what CI and the Pages deploy run)
docs-build:
	cd docs && bun install --frozen-lockfile && bun --bun run build

clean:
	rm -rf bin/ dist/
