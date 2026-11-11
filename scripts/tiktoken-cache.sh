#!/usr/bin/env bash
# Seed a TIKTOKEN_CACHE_DIR with checksum-pinned BPE encodings.
#
# tiktoken-go caches each encoding at <cache_dir>/<sha1(url)> and only fetches
# over the network on a cache miss. Pre-seeding the directory lets tests,
# containers and offline QEMU microVMs count tokens without network access.
# Copy the seeded directory into an image and set TIKTOKEN_CACHE_DIR to use it.
#
# Usage: scripts/tiktoken-cache.sh <cache_dir>
set -euo pipefail

cache_dir="${1:?usage: $0 <cache_dir>}"
base_url="https://openaipublic.blob.core.windows.net/encodings"

# name sha256 (same pins as OpenAI's tiktoken openai_public.py)
encodings=(
	"cl100k_base 223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7"
	"o200k_base 446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d"
)

mkdir -p "$cache_dir"

for entry in "${encodings[@]}"; do
	read -r name want <<<"$entry"
	url="$base_url/$name.tiktoken"
	key="$(printf '%s' "$url" | sha1sum | cut -d' ' -f1)"
	dest="$cache_dir/$key"

	if [[ -f "$dest" ]] && [[ "$(sha256sum "$dest" | cut -d' ' -f1)" == "$want" ]]; then
		continue
	fi

	echo "tiktoken-cache: fetching $name"
	tmp="$(mktemp "$cache_dir/.$name.XXXXXX")"
	trap 'rm -f "$tmp"' EXIT
	curl -fsSL --retry 3 -o "$tmp" "$url"
	got="$(sha256sum "$tmp" | cut -d' ' -f1)"
	if [[ "$got" != "$want" ]]; then
		echo "tiktoken-cache: $name checksum mismatch: got $got, want $want" >&2
		exit 1
	fi
	mv "$tmp" "$dest"
	trap - EXIT
done
