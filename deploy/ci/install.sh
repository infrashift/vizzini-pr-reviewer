#!/usr/bin/env bash
# install.sh: install vizzini-bot, mrman and garmr for one CI job, every
# piece pinned and verified.
#
#   deploy/ci/install.sh TOOLS_DIR
#
# Run it from a Vizzini checkout at the commit the pipeline pins (the CI
# templates check one out). It writes:
#
#   TOOLS_DIR/bin/vizzini-bot   the $VIZZINI_VERSION release, checked against
#                               the release's SHA256SUMS and against this
#                               checkout's commit; without VIZZINI_VERSION,
#                               built from this checkout (CGO_ENABLED=0)
#   TOOLS_DIR/bin/garmr         go install of infrashift/garmr at $GARMR_REF
#   TOOLS_DIR/bin/mrman         the $MRMAN_VERSION release, checked against
#                               the release's SHA256SUMS
#   TOOLS_DIR/tiktoken          the tokenizer encodings, checked against
#                               pinned SHA-256 sums (scripts/tiktoken-cache.sh)
#
# Then put TOOLS_DIR/bin on PATH and set TIKTOKEN_CACHE_DIR=TOOLS_DIR/tiktoken.
# Needs go (any version from Vizzini's go.mod on; newer toolchains a module
# requires are fetched), curl, tar and sha256sum. Vizzini releases are for
# Linux on amd64 and arm64 (ADR-0025); elsewhere, leave VIZZINI_VERSION unset.
set -euo pipefail

VIZZINI_VERSION="${VIZZINI_VERSION:-}"
MRMAN_VERSION="${MRMAN_VERSION:-v0.1.9}"
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

goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# fetch_release URL NAME: download NAME.tar.gz and SHA256SUMS from the release
# at URL into $tmp/NAME, check the archive against SHA256SUMS, and extract it.
fetch_release() {
	local url="$1" name="$2" dir="$tmp/$2"
	mkdir -p "$dir"
	curl -fsSL --retry 3 -o "$dir/$name.tar.gz" "$url/$name.tar.gz"
	curl -fsSL --retry 3 -o "$dir/SHA256SUMS" "$url/SHA256SUMS"
	grep -q "  $name.tar.gz\$" "$dir/SHA256SUMS" || die "$name.tar.gz is not in the release's SHA256SUMS"
	(cd "$dir" && grep "  $name.tar.gz\$" SHA256SUMS | sha256sum -c --quiet -) || die "$name.tar.gz does not match SHA256SUMS"
	tar -xzf "$dir/$name.tar.gz" -C "$dir"
}

if [[ -n "$VIZZINI_VERSION" ]]; then
	[[ "$goos" == linux ]] || die "Vizzini releases are for Linux only; unset VIZZINI_VERSION to build vizzini-bot from this checkout"
	name="vizzini_${VIZZINI_VERSION#v}_${goos}_${goarch}"
	echo "install: vizzini-bot $VIZZINI_VERSION ($name)"
	fetch_release "https://github.com/infrashift/vizzini-pr-reviewer/releases/download/$VIZZINI_VERSION" "$name"
	install -m 0755 "$tmp/$name/$name/vizzini-bot" "$bin/vizzini-bot"
	# The release must be the commit this checkout is at: the templates pin
	# both, and the scripts here must match the binary.
	if head="$(git -C "$src" rev-parse HEAD 2>/dev/null)"; then
		"$bin/vizzini-bot" --version | grep -q "(commit $head," ||
			die "vizzini-bot $VIZZINI_VERSION is not built from this checkout ($head); pin VIZZINI_REF to the release's commit"
	fi
else
	echo "install: vizzini-bot from $(git -C "$src" rev-parse --short HEAD 2>/dev/null || echo "$src")"
	(cd "$src" && go build -trimpath -o "$bin/vizzini-bot" ./cmd/vizzini-bot)
fi
"$bin/vizzini-bot" --version

echo "install: garmr at $GARMR_REF"
GOBIN="$bin" go install "github.com/infrashift/garmr/cmd/garmr@$GARMR_REF"

name="mrman_${MRMAN_VERSION}_${goos}_${goarch}"
echo "install: mrman $MRMAN_VERSION ($name)"
fetch_release "https://github.com/infrashift/mrman/releases/download/$MRMAN_VERSION" "$name"
install -m 0755 "$tmp/$name/$name/mrman" "$bin/mrman"

echo "install: tokenizer encodings"
"$src/scripts/tiktoken-cache.sh" "$tools/tiktoken" >/dev/null

"$bin/mrman" --version
echo "install: done; PATH=$bin:\$PATH TIKTOKEN_CACHE_DIR=$tools/tiktoken"
