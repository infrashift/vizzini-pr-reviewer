#!/usr/bin/env bash
# install.sh: install vizzini-bot, mrman and garmr for one CI job, every
# piece pinned and verified.
#
#   deploy/ci/install.sh TOOLS_DIR
#
# Run it from a Vizzini checkout at the commit the pipeline pins (the CI
# templates check one out). It writes:
#
#   TOOLS_DIR/bin/vizzini-bot   built from this checkout (CGO_ENABLED=0)
#   TOOLS_DIR/bin/garmr         go install of infrashift/garmr at $GARMR_REF
#   TOOLS_DIR/bin/mrman         the $MRMAN_VERSION release, checked against
#                               the release's SHA256SUMS
#   TOOLS_DIR/tiktoken          the tokenizer encodings, checked against
#                               pinned SHA-256 sums (scripts/tiktoken-cache.sh)
#
# Then put TOOLS_DIR/bin on PATH and set TIKTOKEN_CACHE_DIR=TOOLS_DIR/tiktoken.
# Needs go (any version from Vizzini's go.mod on; newer toolchains a module
# requires are fetched), curl, tar and sha256sum.
set -euo pipefail

MRMAN_VERSION="${MRMAN_VERSION:-v0.1.7}"
GARMR_REF="${GARMR_REF:-bf96753b9123fc9ac667afed84eba1610d896c53}"

die() { printf 'install: %s\n' "$*" >&2; exit 1; }
[[ $# -eq 1 ]] || die "usage: $0 TOOLS_DIR"
src="$(cd "$(dirname "$0")/../.." && pwd)"
mkdir -p "$1/bin"
tools="$(cd "$1" && pwd)"
bin="$tools/bin"
export CGO_ENABLED=0
# Each module may require a newer Go than the one installed (garmr needs
# 1.26.11): let go fetch that toolchain, which it verifies against the Go
# checksum database. CI setups often pin GOTOOLCHAIN=local, which would fail.
export GOTOOLCHAIN=auto

echo "install: vizzini-bot from $(git -C "$src" rev-parse --short HEAD 2>/dev/null || echo "$src")"
(cd "$src" && go build -trimpath -o "$bin/vizzini-bot" ./cmd/vizzini-bot)

echo "install: garmr at $GARMR_REF"
GOBIN="$bin" go install "github.com/infrashift/garmr/cmd/garmr@$GARMR_REF"

name="mrman_${MRMAN_VERSION}_$(go env GOOS)_$(go env GOARCH)"
url="https://github.com/infrashift/mrman/releases/download/$MRMAN_VERSION"
echo "install: mrman $MRMAN_VERSION ($name)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL --retry 3 -o "$tmp/$name.tar.gz" "$url/$name.tar.gz"
curl -fsSL --retry 3 -o "$tmp/SHA256SUMS" "$url/SHA256SUMS"
grep -q "  $name.tar.gz\$" "$tmp/SHA256SUMS" || die "$name.tar.gz is not in the release's SHA256SUMS"
(cd "$tmp" && grep "  $name.tar.gz\$" SHA256SUMS | sha256sum -c --quiet -) || die "$name.tar.gz does not match SHA256SUMS"
tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
install -m 0755 "$tmp/$name/mrman" "$bin/mrman"

echo "install: tokenizer encodings"
"$src/scripts/tiktoken-cache.sh" "$tools/tiktoken" >/dev/null

"$bin/mrman" --version
echo "install: done; PATH=$bin:\$PATH TIKTOKEN_CACHE_DIR=$tools/tiktoken"
