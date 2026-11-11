#!/usr/bin/env bash
# End-to-end proof of lint-aware Go review (Milestone 4.2), using real tools:
#
#   1. a throwaway Go repo with a base commit holding a PRE-EXISTING lint issue
#   2. a change that adds NEW lint issues (errcheck, gosec SQL concatenation)
#   3. real golangci-lint v2 JSON, run from the repo root
#   4. the real `git diff` and the real bin/pr-review binary
#
# Model: if VIZZINI_LITELLM_URL and VIZZINI_MODEL are set, your LiteLLM proxy
# answers (assertions are then limited to what does not depend on the model).
# Otherwise scripts/e2e/stubllm answers with one finding per diagnostic in the
# prompt's <lint> block, so every finding proves a diagnostic made the trip.
#
# Usage: scripts/e2e-golint.sh   (or: make e2e-lint)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
golangci="${GOLANGCI:-$(go env GOPATH)/bin/golangci-lint}"
prreview="$root/bin/pr-review"
export TIKTOKEN_CACHE_DIR="${TIKTOKEN_CACHE_DIR:-$root/.cache/tiktoken}"

work="$(mktemp -d)"
stub_pid=""
cleanup() {
	[[ -n "$stub_pid" ]] && kill "$stub_pid" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT

pass() { printf '  \033[32mPASS\033[0m %s\n' "$1"; }
fail() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; exit 1; }

[[ -x "$prreview" ]] || fail "bin/pr-review missing; run make build"
[[ -x "$golangci" ]] || fail "golangci-lint not found at $golangci (set GOLANGCI)"

echo "== 1. throwaway repo with a pre-existing lint issue"
repo="$work/repo"
mkdir -p "$repo/store"
cd "$repo"
git init -q -b main
printf 'module example.com/e2e\n\ngo 1.25\n' >go.mod
cat >.golangci.yml <<'EOF'
version: "2"
linters:
  enable: [gosec]
EOF
cat >store/users.go <<'EOF'
package store

import (
	"database/sql"
	"os"
)

// Legacy has a lint issue (unchecked Close) that the change below does not touch.
func Legacy(path string) {
	f, _ := os.Open(path)
	f.Close()
}

// FindUser looks a user up by name.
func FindUser(db *sql.DB, name string) *sql.Row {
	return db.QueryRow("SELECT id FROM users WHERE name = $1", name)
}
EOF
git add -A
git -c user.name=e2e -c user.email=e2e@example.invalid commit -q -m base

echo "== 2. a change that adds lint issues"
cat >store/users.go <<'EOF'
package store

import (
	"database/sql"
	"os"
)

// Legacy has a lint issue (unchecked Close) that the change below does not touch.
func Legacy(path string) {
	f, _ := os.Open(path)
	f.Close()
}

// FindUser looks a user up by name.
func FindUser(db *sql.DB, name string) *sql.Row {
	q := "SELECT id FROM users WHERE name = '" + name + "'"
	return db.QueryRow(q)
}

// Save writes data to path.
func Save(path string, data []byte) {
	f, _ := os.Create(path)
	f.Write(data)
}
EOF
git diff >"$work/change.patch"
echo "   $(grep -c '^+[^+]' "$work/change.patch") added line(s) in the diff"

echo "== 3. real golangci-lint JSON from the repo root"
# Stats are left on deliberately: golangci-lint v2 then appends a text
# summary ("2 issues: ...") after the JSON on stdout, the messier real-world
# shape. Vizzini reads only the first JSON value, so it must still work.
"$golangci" run --output.json.path=stdout --output.text.path=/dev/null ./... >"$work/lint.json" || true
python3 -I - "$work/lint.json" <<'EOF'
import json, sys
issues = json.JSONDecoder().raw_decode(open(sys.argv[1]).read())[0]["Issues"] or []
for i in issues:
    p = i["Pos"]
    print(f'   {p["Filename"]}:{p["Line"]} {i["FromLinter"]}: {i["Text"]}')
EOF

echo "== 4. pr-review"
if [[ -n "${VIZZINI_LITELLM_URL:-}" && -n "${VIZZINI_MODEL:-}" ]]; then
	mode=real
	echo "   model: $VIZZINI_MODEL via $VIZZINI_LITELLM_URL"
else
	mode=stub
	addr="127.0.0.1:${STUB_PORT:-18765}"
	(cd "$root" && go build -o "$work/stubllm" ./scripts/e2e/stubllm)
	"$work/stubllm" -addr "$addr" -log "$work/prompts.log" 2>"$work/stubllm.err" &
	stub_pid=$!
	for _ in $(seq 50); do curl -s -o /dev/null "http://$addr/" && break; sleep 0.1; done
	export VIZZINI_LITELLM_URL="http://$addr/v1" VIZZINI_MODEL=stub
	echo "   model: stubllm (no VIZZINI_LITELLM_URL set)"
fi

set +e
(cd "$repo" && "$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$work/lint.json" --format json) >"$work/review.json" 2>"$work/stderr"
code=$?
(cd "$repo" && "$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$work/lint.json" --fail-on none) >"$work/review.md" 2>/dev/null
set -e
echo "   exit code $code; stderr: $(cat "$work/stderr")"

echo "== 5. assertions"
python3 -I "$root/scripts/e2e/assert_lint.py" "$work/lint.json" "$work/change.patch" "$work/review.json" "$mode" "$code" "$work/prompts.log" "$repo"

echo "== 6. Markdown review (as mrman would render it)"
sed 's/^/   | /' "$work/review.md"
