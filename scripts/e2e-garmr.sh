#!/usr/bin/env bash
# End-to-end proof of the garmr integration (ISSUE-011, ADR-0010):
#
#   real bin/pr-review  ->  JSON report (apiVersion/kind envelope)
#                       ->  real garmr-server with policies/garmr loaded
#                       ->  garmr eval -n vizzini  ->  allow / warn / deny
#
# The model is scripts/e2e/stubllm (one finding per <lint> entry), so each
# scenario's findings are fixed by its lint input.
#
# garmr and garmr-server are found in $GARMR_BIN_DIR or on PATH. Without
# them the script prints SKIP and exits 0, since garmr is optional.
#
# Usage: scripts/e2e-garmr.sh   (or: make e2e-garmr)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prreview="$root/bin/pr-review"
export TIKTOKEN_CACHE_DIR="${TIKTOKEN_CACHE_DIR:-$root/.cache/tiktoken}"

find_bin() {
	if [[ -n "${GARMR_BIN_DIR:-}" && -x "$GARMR_BIN_DIR/$1" ]]; then
		echo "$GARMR_BIN_DIR/$1"
	else
		command -v "$1" || true
	fi
}
garmr="$(find_bin garmr)"
garmr_server="$(find_bin garmr-server)"
if [[ -z "$garmr" || -z "$garmr_server" ]]; then
	echo "SKIP: garmr/garmr-server not found (set GARMR_BIN_DIR or add them to PATH)"
	exit 0
fi

work="$(mktemp -d)"
pids=()
cleanup() {
	for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
	rm -rf "$work"
}
trap cleanup EXIT

pass() { printf '  \033[32mPASS\033[0m %s\n' "$1"; }
fail() {
	printf '  \033[31mFAIL\033[0m %s\n' "$1"
	exit 1
}

[[ -x "$prreview" ]] || fail "bin/pr-review missing; run make build"

echo "== 1. policy pack: validate and offline tests"
"$garmr" validate "$root/policies/garmr/vizzini" >/dev/null || fail "garmr validate"
for t in review-gate review-advisory; do
	"$garmr" test "$root/policies/garmr/vizzini/review.cue" "$root/policies/garmr/vizzini/$t""_test.cue" >"$work/$t.test" 2>&1 ||
		{ cat "$work/$t.test"; fail "garmr test $t"; }
	pass "garmr test $t: $(grep -o 'Results: .*' "$work/$t.test")"
done

echo "== 2. garmr-server with policies/garmr"
gport="${GARMR_PORT:-18767}"
GARMR_HTTP_ADDR="127.0.0.1:$gport" "$garmr_server" --policy-dir "$root/policies/garmr" --audit-path "$work/audit.log" >"$work/garmr-server.log" 2>&1 &
pids+=($!)
for _ in $(seq 50); do curl -s -o /dev/null "http://127.0.0.1:$gport/v1/policies" && break; sleep 0.1; done
curl -s -o /dev/null "http://127.0.0.1:$gport/v1/policies" || { cat "$work/garmr-server.log"; fail "garmr-server did not start"; }
pass "garmr-server listening on 127.0.0.1:$gport"

echo "== 3. stand-in model"
sport="${STUB_PORT:-18768}"
(cd "$root" && go build -o "$work/stubllm" ./scripts/e2e/stubllm)
"$work/stubllm" -addr "127.0.0.1:$sport" 2>"$work/stubllm.log" &
pids+=($!)
for _ in $(seq 50); do curl -s -o /dev/null "http://127.0.0.1:$sport/" && break; sleep 0.1; done
export VIZZINI_LITELLM_URL="http://127.0.0.1:$sport/v1" VIZZINI_MODEL=stub

# new_file_patch PATH: a diff adding a five-line Go file at PATH.
new_file_patch() {
	cat <<EOF
diff --git a/$1 b/$1
new file mode 100644
--- /dev/null
+++ b/$1
@@ -0,0 +1,5 @@
+package x
+
+func Find(db DB, name string) Row {
+	return db.QueryRow("SELECT id FROM users WHERE name = '" + name + "'")
+}
EOF
}

# scenario NAME PATH LINT_JSON WANT_DECISION [WANT_RULE]
scenario() {
	local name=$1 path=$2 lint=$3 want=$4 rule=${5:-}
	new_file_patch "$path" >"$work/$name.patch"
	printf '%s' "$lint" >"$work/$name.lint.json"
	"$prreview" --mode review --fail-on none --format json \
		--patch-file "$work/$name.patch" --lint-json "$work/$name.lint.json" >"$work/$name.review.json" ||
		fail "$name: pr-review failed"
	set +e
	"$garmr" eval --server "http://127.0.0.1:$gport" --input "$work/$name.review.json" -n vizzini -o json >"$work/$name.eval.json" 2>"$work/$name.eval.err"
	set -e
	python3 -I - "$work/$name.review.json" "$work/$name.eval.json" "$want" "$rule" <<'PY' || fail "$name"
import json, sys
review = json.load(open(sys.argv[1]))
ev = json.load(open(sys.argv[2]))
want, rule = sys.argv[3], sys.argv[4]
assert review["apiVersion"] == "vizzini.infrashift.io/v1" and review["kind"] == "Review", "report lacks the envelope"
failed = sorted(r["rule_id"] for r in ev.get("results", []) if not r["passed"])
print(f'    decision={ev["decision"]} failed={failed} risk={review["risk_score"]} findings={[f["severity"] for f in review["findings"]]}')
if ev["decision"] != want:
    sys.exit(f"decision {ev['decision']}, want {want}")
if rule and rule not in failed:
    sys.exit(f"{rule} did not fire")
if not rule and failed:
    sys.exit(f"unexpected failed rules {failed}")
PY
	pass "$name -> $want${rule:+ ($rule)}"
}

echo "== 4. pr-review report -> garmr eval"
gosec='{"Issues":[{"FromLinter":"gosec","Text":"G202: SQL string concatenation","Pos":{"Filename":"internal/store/users.go","Line":4,"Column":2}}]}'
none='{"Issues":[]}'
scenario risky internal/store/users.go "$gosec" deny VZ-001
scenario clean internal/api/list.go "$none" allow
scenario sensitive internal/auth/token.go "$none" warn VZA-002
