#!/usr/bin/env bash
# End-to-end proof of the garmr integration (ISSUE-011, ADR-0010):
#
#   real bin/pr-review  ->  JSON report (apiVersion/kind envelope)
#                       ->  garmr eval --policy-dir policies/garmr -n vizzini
#                           (in-process: no garmr-server)
#                       ->  allow / warn / deny, and garmr's exit code
#
# The model is scripts/e2e/stubllm (one finding per <lint> entry), so each
# scenario's findings are fixed by its lint input.
#
# garmr is found in $GARMR_BIN_DIR or on PATH and must support local
# evaluation (`garmr eval --policy-dir`). Without it the script prints SKIP
# and exits 0, since garmr is optional.
#
# Usage: scripts/e2e-garmr.sh   (or: make e2e-garmr)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prreview="$root/bin/pr-review"
policies="$root/policies/garmr"
export TIKTOKEN_CACHE_DIR="${TIKTOKEN_CACHE_DIR:-$root/.cache/tiktoken}"

if [[ -n "${GARMR_BIN_DIR:-}" && -x "$GARMR_BIN_DIR/garmr" ]]; then
	garmr="$GARMR_BIN_DIR/garmr"
else
	garmr="$(command -v garmr || true)"
fi
if [[ -z "$garmr" ]]; then
	echo "SKIP: garmr not found (set GARMR_BIN_DIR or add it to PATH)"
	exit 0
fi
if ! "$garmr" eval --help 2>/dev/null | grep -q -- '--policy-dir'; then
	echo "SKIP: $garmr has no local evaluation (garmr eval --policy-dir); upgrade garmr"
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
"$garmr" validate "$policies/vizzini" >/dev/null || fail "garmr validate"
for t in review-gate review-advisory; do
	"$garmr" test "$policies/vizzini/review.cue" "$policies/vizzini/${t}_test.cue" >"$work/$t.test" 2>&1 ||
		{ cat "$work/$t.test"; fail "garmr test $t"; }
	pass "garmr test $t: $(grep -o 'Results: .*' "$work/$t.test")"
done

echo "== 2. stand-in model"
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

# scenario NAME PATH LINT_JSON WANT_DECISION [WANT_RULES]
# WANT_RULES is the exact comma-separated set of rules that must fail.
scenario() {
	local name=$1 path=$2 lint=$3 want=$4 rules=${5:-}
	new_file_patch "$path" >"$work/$name.patch"
	printf '%s' "$lint" >"$work/$name.lint.json"
	"$prreview" --mode review --fail-on none --format json \
		--patch-file "$work/$name.patch" --lint-json "$work/$name.lint.json" >"$work/$name.review.json" ||
		fail "$name: pr-review failed"
	set +e
	"$garmr" eval --policy-dir "$policies" --input "$work/$name.review.json" -n vizzini \
		--request-id "vizzini-e2e#$name@local" -o json >"$work/$name.eval.json" 2>"$work/$name.eval.err"
	local code=$?
	set -e
	[[ $code -ne 2 ]] || { cat "$work/$name.eval.err"; fail "$name: garmr eval could not run"; }
	python3 -I - "$work/$name.review.json" "$work/$name.eval.json" "$want" "$rules" "$code" <<'PY' || fail "$name"
import json, sys
review = json.load(open(sys.argv[1]))
ev = json.load(open(sys.argv[2]))
want, rules, code = sys.argv[3], sys.argv[4], int(sys.argv[5])
assert review["apiVersion"] == "vizzini.infrashift.io/v1" and review["kind"] == "Review", "report lacks the envelope"
failed = sorted(r["rule_id"] for r in ev.get("results", []) if not r["passed"])
print(f'    decision={ev["decision"]} exit={code} failed={failed} risk={review["risk_score"]} '
      f'risk_files={"absent" if "risk_files" not in review else len(review["risk_files"])} '
      f'findings={[f["severity"] for f in review["findings"]]}')
if ev["decision"] != want:
    sys.exit(f"decision {ev['decision']}, want {want}")
if failed != sorted(filter(None, rules.split(","))):
    sys.exit(f"failed rules {failed}, want [{rules}]")
# garmr: 1 = deny, 0 = allow or warn.
if code != (1 if want == "deny" else 0):
    sys.exit(f"garmr exit {code} for {want}")
PY
	pass "$name -> $want, exit $code${rules:+ ($rules)}"
}

echo "== 3. pr-review report -> garmr eval --policy-dir (no server)"
gosec='{"Issues":[{"FromLinter":"gosec","Text":"G202: SQL string concatenation","Pos":{"Filename":"internal/store/users.go","Line":4,"Column":2}}]}'
none='{"Issues":[]}'
scenario risky internal/store/users.go "$gosec" deny VZ-001
scenario clean internal/api/list.go "$none" allow
scenario sensitive internal/auth/token.go "$none" warn VZA-002
# Only an excluded lock file changes: nothing is reviewed, so the report has
# no risk_files. VZA-002 must treat the absent list as empty (allowMissing);
# only VZA-003 fires.
scenario lockfile go.sum "$none" warn VZA-003
