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
python3 -I - "$work/lint.json" "$work/change.patch" "$work/review.json" "$mode" "$code" "${work}/prompts.log" <<'EOF'
import json, os, re, sys
lint_path, patch_path, review_path, mode, code, prompts_path = sys.argv[1:]
code = int(code)

def ok(msg): print(f"  \033[32mPASS\033[0m {msg}")
def die(msg):
    print(f"  \033[31mFAIL\033[0m {msg}"); sys.exit(1)

# Independently derive the added new-side lines from the raw patch.
added, new = set(), 0
for line in open(patch_path):
    m = re.match(r"@@ -\d+(?:,\d+)? \+(\d+)", line)
    if m:
        new = int(m.group(1)); continue
    if line.startswith("+++") or line.startswith("---"):
        continue
    if line.startswith("+"):
        added.add(new); new += 1
    elif line.startswith(" "):
        new += 1

issues = json.JSONDecoder().raw_decode(open(lint_path).read())[0]["Issues"] or []
on_added = [i for i in issues if i["Pos"]["Line"] in added]
untouched = [i for i in issues if i["Pos"]["Line"] not in added]
if not on_added: die("golangci-lint reported nothing on added lines; the fixture is broken")
if not untouched: die("expected the pre-existing Legacy issue on an untouched line")
ok(f"golangci-lint: {len(on_added)} issue(s) on added lines, {len(untouched)} on untouched lines")

review = json.loads(open(review_path).read())
if review.get("diagnostics") != len(on_added):
    die(f'diagnostics={review.get("diagnostics")}, want {len(on_added)}')
ok(f'pr-review injected exactly the {len(on_added)} changed-line diagnostic(s)')

diff_files = set(re.findall(r"^\+\+\+ b/(\S+)$", open(patch_path).read(), re.M))
for f in review["findings"]:
    if f["file"] not in diff_files: die(f"finding outside the diff: {f}")
    if not any(f["start_line"] <= ln <= f["end_line"] for ln in added) and mode == "stub":
        die(f"stub finding not on an added line: {f}")
if review["chunks"] < 1: die("nothing was reviewed")
ok(f'{len(review["findings"])} finding(s), all schema-validated and anchored to the diff')

if mode == "stub":
    prompts = open(prompts_path).read()
    for i in on_added:
        needle = f'{i["Pos"]["Filename"]}:{i["Pos"]["Line"]}:'
        if needle not in prompts: die(f"prompt lacks changed-line diagnostic {needle}")
    for i in untouched:
        needle = f'{i["Pos"]["Filename"]}:{i["Pos"]["Line"]}:'
        if needle in prompts.split("<lint>")[-1].split("</lint>")[0]:
            die(f"prompt <lint> block leaked untouched-line diagnostic {needle}")
    ok("prompt <lint> block holds the new issues and none of the pre-existing ones")
    if len(review["findings"]) != len(on_added): die("stub should echo one finding per diagnostic")
    want = 2 if any(i["FromLinter"] == "gosec" for i in on_added) else 0
    if code != want: die(f"exit code {code}, want {want} (gosec maps to Major, which the default gate blocks)")
    ok(f"policy gate: exit {code} (Major security finding blocks by default)")
else:
    if code not in (0, 2): die(f"exit code {code}")
    ok(f"real model run completed (exit {code})")

if review["auto_approve"]:
    die("auto_approve must be false: the change has security findings or a risky diff")
ok(f'risk {review["risk_score"]} (diff {review["base_risk_score"]}, model {review["model_risk_score"]}), auto_approve false')
EOF

echo "== 6. Markdown review (as mrman would render it)"
sed 's/^/   | /' "$work/review.md"
