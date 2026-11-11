#!/usr/bin/env bash
# End-to-end proof of lint-aware Python review via SARIF (ISSUE-014):
#
#   1. a throwaway Python repo whose base commit has PRE-EXISTING ruff issues
#   2. a change that adds NEW ones (unused import, string-built SQL)
#   3. real ruff SARIF (absolute file:// URIs), run from the repo root
#   4. the real `git diff` and bin/pr-review, run from the repo root so
#      the URIs resolve against it
#
# ruff comes from $RUFF, or `ruff` on PATH, or `uvx ruff`; without any of
# them the script prints SKIP and exits 0. The model is your LiteLLM proxy if
# VIZZINI_LITELLM_URL/VIZZINI_MODEL are set, else scripts/e2e/stubllm.
#
# SARIF_FIXTURE_OUT=path also saves the captured SARIF as a unit-test fixture,
# with the temp repo path rewritten to file:///work/repo/.
#
# Usage: scripts/e2e-pylint.sh   (or: make e2e-lint-python)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prreview="$root/bin/pr-review"
export TIKTOKEN_CACHE_DIR="${TIKTOKEN_CACHE_DIR:-$root/.cache/tiktoken}"

if [[ -n "${RUFF:-}" ]]; then
	read -r -a ruff <<<"$RUFF"
elif command -v ruff >/dev/null; then
	ruff=(ruff)
elif command -v uvx >/dev/null; then
	ruff=(uvx ruff)
else
	echo "SKIP: ruff not found (install ruff or uv, or set RUFF)"
	exit 0
fi

work="$(mktemp -d)"
stub_pid=""
cleanup() {
	[[ -n "$stub_pid" ]] && kill "$stub_pid" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT

fail() {
	printf '  \033[31mFAIL\033[0m %s\n' "$1"
	exit 1
}
[[ -x "$prreview" ]] || fail "bin/pr-review missing; run make build"

echo "== 1. throwaway repo with pre-existing ruff issues"
repo="$work/repo"
mkdir -p "$repo/app"
cd "$repo"
git init -q -b main
cat >pyproject.toml <<'EOF'
[tool.ruff.lint]
select = ["F", "S"]
EOF
cat >app/views.py <<'EOF'
import json


def legacy(path):
    return eval(open(path).read())


def find(db, uid):
    return db.execute("SELECT * FROM users WHERE id = %s", (uid,))
EOF
git add -A
git -c user.name=e2e -c user.email=e2e@example.invalid commit -q -m base

echo "== 2. a change that adds ruff issues"
cat >app/views.py <<'EOF'
import json
import os


def legacy(path):
    return eval(open(path).read())


def find(db, uid):
    query = "SELECT * FROM users WHERE id = " + uid
    return db.execute(query)
EOF
git diff >"$work/change.patch"
echo "   $(grep -c '^+[^+]' "$work/change.patch") added line(s) in the diff"

echo "== 3. real ruff SARIF from the repo root"
echo "   $("${ruff[@]}" --version)"
"${ruff[@]}" check --output-format=sarif . >"$work/ruff.sarif" || true
python3 -I - "$work/ruff.sarif" <<'EOF'
import json, sys
for run in json.load(open(sys.argv[1]))["runs"]:
    for r in run["results"]:
        loc = r["locations"][0]["physicalLocation"]
        print(f'   {loc["artifactLocation"]["uri"].split("/repo/")[-1]}:{loc["region"]["startLine"]} {r["ruleId"]}: {r["message"]["text"]}')
EOF

if [[ -n "${SARIF_FIXTURE_OUT:-}" ]]; then
	# Keep the real output, but replace this machine's temp path with a
	# neutral root so the fixture is reproducible and leaks nothing local.
	sed "s#file://$repo/#file:///work/repo/#g" "$work/ruff.sarif" >"$SARIF_FIXTURE_OUT"
	echo "   fixture written to $SARIF_FIXTURE_OUT (root rewritten to /work/repo)"
fi

echo "== 4. pr-review"
if [[ -n "${VIZZINI_LITELLM_URL:-}" && -n "${VIZZINI_MODEL:-}" ]]; then
	mode=real
	echo "   model: $VIZZINI_MODEL via $VIZZINI_LITELLM_URL"
else
	mode=stub
	addr="127.0.0.1:${STUB_PORT:-18769}"
	(cd "$root" && go build -o "$work/stubllm" ./scripts/e2e/stubllm)
	"$work/stubllm" -addr "$addr" -log "$work/prompts.log" 2>"$work/stubllm.err" &
	stub_pid=$!
	for _ in $(seq 50); do curl -s -o /dev/null "http://$addr/" && break; sleep 0.1; done
	export VIZZINI_LITELLM_URL="http://$addr/v1" VIZZINI_MODEL=stub
	echo "   model: stubllm (no VIZZINI_LITELLM_URL set)"
fi

set +e
"$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$work/ruff.sarif" --format json >"$work/review.json" 2>"$work/stderr"
code=$?
"$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$work/ruff.sarif" --fail-on none >"$work/review.md" 2>/dev/null
set -e
echo "   exit code $code; stderr: $(cat "$work/stderr")"

echo "== 5. assertions"
python3 -I "$root/scripts/e2e/assert_lint.py" "$work/ruff.sarif" "$work/change.patch" "$work/review.json" "$mode" "$code" "$work/prompts.log" "$repo"

echo "== 6. Markdown review"
sed 's/^/   | /' "$work/review.md"
