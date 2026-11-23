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
#   TOOLS_DIR/bin/garmr         the $GARMR_VERSION release, checked against
#                               the release's SHA256SUMS
#   TOOLS_DIR/bin/mrman         the $MRMAN_VERSION release, checked against
#                               the release's SHA256SUMS
#   TOOLS_DIR/tiktoken          the tokenizer encodings, checked against
#                               pinned SHA-256 sums (scripts/tiktoken-cache.sh)
#
# Then put TOOLS_DIR/bin on PATH and set TIKTOKEN_CACHE_DIR=TOOLS_DIR/tiktoken.
#
# On an agent that runs many jobs, set VIZZINI_TOOLS_CACHE to a directory
# that stays between jobs. Each verified release binary is kept there under
# its pin, and later jobs copy it from there and skip the download. A cached binary is used only if it still matches the SHA-256 that
# was recorded when it was cached. Entries never change, so jobs that share
# the cache with other pins do not interfere. The cache is only as
# trustworthy as the agent: any job that runs as the agent's user can write
# it. The recorded SHA-256 catches a damaged file, not a changed one.
#
# Needs curl 7.71 or newer, tar and sha256sum. curl retries each download up
# to three times on any error. Vizzini and mrman releases are for Linux on
# amd64 and arm64 (ADR-0025). Elsewhere, leave VIZZINI_VERSION unset: the
# script then builds vizzini-bot from this checkout, which needs go (the
# version in Vizzini's go.mod or newer); the build is retried up to three
# times.
set -euo pipefail

VIZZINI_VERSION="${VIZZINI_VERSION:-}"
MRMAN_VERSION="${MRMAN_VERSION:-v0.1.10}"
GARMR_VERSION="${GARMR_VERSION:-v0.1.0}"

die() { printf 'install: %s\n' "$*" >&2; exit 1; }

# retry CMD...: run CMD up to three times, 10 s and then 20 s apart. Go does
# not retry a module or toolchain download that loses its connection.
retry() {
	local n
	for n in 1 2 3; do
		"$@" && return 0
		[[ $n -lt 3 ]] || return 1
		printf 'install: attempt %d of 3 failed (%s); retrying in %d s\n' "$n" "$*" $((n * 10)) >&2
		sleep $((n * 10))
	done
}
[[ $# -eq 1 ]] || die "usage: $0 TOOLS_DIR"
[[ -z "${GARMR_REF:-}" ]] || echo "install: GARMR_REF is no longer used; garmr comes from the $GARMR_VERSION release (set GARMR_VERSION to pin another)" >&2
src="$(cd "$(dirname "$0")/../.." && pwd)"
mkdir -p "$1/bin"
tools="$(cd "$1" && pwd)"
bin="$tools/bin"

# The release archives are named with Go's names for the OS and architecture.
goos="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
x86_64 | amd64) goarch=amd64 ;;
aarch64 | arm64) goarch=arm64 ;;
*) goarch="$(uname -m)" ;;
esac
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cache="${VIZZINI_TOOLS_CACHE:-}"

# fetch_release URL NAME: download NAME.tar.gz and SHA256SUMS from the release
# at URL into $tmp/NAME, check the archive against SHA256SUMS, and extract it.
fetch_release() {
	local url="$1" name="$2" dir="$tmp/$2"
	mkdir -p "$dir"
	curl -fsSL --retry 3 --retry-all-errors -o "$dir/$name.tar.gz" "$url/$name.tar.gz"
	curl -fsSL --retry 3 --retry-all-errors -o "$dir/SHA256SUMS" "$url/SHA256SUMS"
	grep -q "  $name.tar.gz\$" "$dir/SHA256SUMS" || die "$name.tar.gz is not in the release's SHA256SUMS"
	(cd "$dir" && grep "  $name.tar.gz\$" SHA256SUMS | sha256sum -c --quiet -) || die "$name.tar.gz does not match SHA256SUMS"
	tar -xzf "$dir/$name.tar.gz" -C "$dir"
}

# cached TOOL KEY CMD...: install TOOL into $bin. CMD must write $tmp/TOOL.
# With a cache, an entry $cache/TOOL/KEY whose binary still matches its
# recorded SHA-256 replaces CMD; otherwise CMD runs and its binary becomes
# the entry. A new entry is written to a temporary directory and renamed into
# place, so a job never sees part of an entry, and an existing one never
# changes.
cached() {
	local tool="$1" key="$2" entry new
	shift 2
	if [[ -n "$cache" ]]; then
		entry="$cache/$tool/$key"
		if [[ -d "$entry" ]]; then
			if (cd "$entry" && sha256sum -c --quiet --strict sha256 >/dev/null 2>&1); then
				echo "install: $tool $key from the cache ($cache)"
				install -m 0755 "$entry/$tool" "$bin/$tool"
				return
			fi
			echo "install: the cached $tool $key does not match its recorded SHA-256; replacing it" >&2
			rm -rf "$entry"
		fi
	fi
	"$@"
	install -m 0755 "$tmp/$tool" "$bin/$tool"
	if [[ -n "$cache" ]]; then
		mkdir -p "$cache/$tool"
		new="$(mktemp -d "$cache/$tool/.new.XXXXXX")"
		install -m 0755 "$tmp/$tool" "$new/$tool"
		(cd "$new" && sha256sum "$tool" >sha256)
		# Another job may have added the same entry first; keep that one.
		mv -T "$new" "$entry" 2>/dev/null || rm -rf "$new"
	fi
}

fetch_vizzini() {
	fetch_release "https://github.com/infrashift/vizzini-pr-reviewer/releases/download/$VIZZINI_VERSION" "$name"
	mv "$tmp/$name/$name/vizzini-bot" "$tmp/vizzini-bot"
}

fetch_garmr() {
	fetch_release "https://github.com/infrashift/garmr/releases/download/$GARMR_VERSION" "$name"
	mv "$tmp/$name/$name/garmr" "$tmp/garmr"
}

fetch_mrman() {
	fetch_release "https://github.com/infrashift/mrman/releases/download/$MRMAN_VERSION" "$name"
	mv "$tmp/$name/$name/mrman" "$tmp/mrman"
}

if [[ -n "$VIZZINI_VERSION" ]]; then
	[[ "$goos" == linux ]] || die "Vizzini releases are for Linux only; unset VIZZINI_VERSION to build vizzini-bot from this checkout"
	name="vizzini_${VIZZINI_VERSION#v}_${goos}_${goarch}"
	echo "install: vizzini-bot $VIZZINI_VERSION ($name)"
	cached vizzini-bot "$name" fetch_vizzini
	# The release must be the commit this checkout is at: the templates pin
	# both, and the scripts here must match the binary. Checked on every run,
	# for a cached binary too.
	if head="$(git -C "$src" rev-parse HEAD 2>/dev/null)"; then
		"$bin/vizzini-bot" --version | grep -q "(commit $head," ||
			die "vizzini-bot $VIZZINI_VERSION is not built from this checkout ($head); pin VIZZINI_REF to the release's commit"
	fi
else
	echo "install: vizzini-bot from $(git -C "$src" rev-parse --short HEAD 2>/dev/null || echo "$src")"
	(cd "$src" && CGO_ENABLED=0 GOTOOLCHAIN=auto retry go build -trimpath -o "$bin/vizzini-bot" ./cmd/vizzini-bot)
fi
"$bin/vizzini-bot" --version

name="garmr_${GARMR_VERSION#v}_${goos}_${goarch}"
echo "install: garmr $GARMR_VERSION ($name)"
cached garmr "$name" fetch_garmr

name="mrman_${MRMAN_VERSION}_${goos}_${goarch}"
echo "install: mrman $MRMAN_VERSION ($name)"
cached mrman "$name" fetch_mrman

echo "install: tokenizer encodings"
# tiktoken-cache.sh checks each file against its pinned SHA-256 and fetches
# only a missing or wrong one.
if [[ -n "$cache" ]]; then
	"$src/scripts/tiktoken-cache.sh" "$cache/tiktoken" >/dev/null
	mkdir -p "$tools/tiktoken"
	cp "$cache/tiktoken/"* "$tools/tiktoken/"
fi
"$src/scripts/tiktoken-cache.sh" "$tools/tiktoken" >/dev/null

"$bin/mrman" --version
"$bin/garmr" version | grep -q "Version: *$GARMR_VERSION\$" || die "garmr does not report $GARMR_VERSION"
echo "garmr $GARMR_VERSION"
echo "install: done; PATH=$bin:\$PATH TIKTOKEN_CACHE_DIR=$tools/tiktoken"
