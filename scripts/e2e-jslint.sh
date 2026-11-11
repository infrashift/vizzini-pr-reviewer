#!/usr/bin/env bash
# End-to-end proof of lint-aware TypeScript/JavaScript review via ESLint
# SARIF (ISSUE-018):
#
#   1. a throwaway TypeScript repo whose base commit has a PRE-EXISTING
#      warning (an unused variable) and an eslint-disable'd eval
#   2. a change that adds NEW ones: eval, string-built setTimeout, new
#      Function, an unused variable, a floating promise (type-aware), and an
#      eval under eslint-disable, which must stay out of the prompt
#   3. real SARIF from ESLint + typescript-eslint through
#      @microsoft/eslint-formatter-sarif, run in the node image ($NODE_IMAGE)
#      under podman with the repo mounted at its own absolute path, so the
#      file:// URIs it writes resolve on the host
#   4. the real `git diff` and bin/pr-review, run from the repo root
#
# Needs podman (or docker via $CONTAINER) and network access for npm on
# first run (packages are cached in .cache/npm). Without a container runtime
# it prints SKIP and exits 0. The model is your LiteLLM proxy if
# VIZZINI_LITELLM_URL/VIZZINI_MODEL are set, else stubllm.
#
# SARIF_FIXTURE_OUT=path also saves the captured SARIF as a unit-test
# fixture, with the temp repo path rewritten to /work/repo.
#
# Usage: scripts/e2e-jslint.sh   (or: make e2e-lint-js)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prreview="$root/bin/pr-review"
export TIKTOKEN_CACHE_DIR="${TIKTOKEN_CACHE_DIR:-$root/.cache/tiktoken}"
container="${CONTAINER:-podman}"
# node 24.22.1 (2026-10-21), pinned by digest.
image="${NODE_IMAGE:-docker.io/library/node:24.22.1-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53}"
npm_cache="$root/.cache/npm"

if ! command -v "$container" >/dev/null; then
	echo "SKIP: $container not found (set CONTAINER=docker to use docker)"
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

echo "== 1. throwaway repo with a pre-existing warning"
repo="$work/repo"
mkdir -p "$repo/src" "$npm_cache"
cd "$repo"
git init -q -b main
printf 'node_modules/\neslint.sarif\n' >.gitignore
cat >package.json <<'EOF'
{
  "name": "e2e-jslint",
  "private": true,
  "type": "module",
  "devDependencies": {
    "@eslint/js": "10.0.1",
    "@microsoft/eslint-formatter-sarif": "3.1.0",
    "eslint": "10.13.0",
    "typescript": "6.0.3",
    "typescript-eslint": "8.72.1"
  }
}
EOF
cat >tsconfig.json <<'EOF'
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "noEmit": true
  },
  "include": ["src"]
}
EOF
cat >eslint.config.mjs <<'EOF'
import js from "@eslint/js";
import { defineConfig } from "eslint/config";
import tseslint from "typescript-eslint";

export default defineConfig(
  { ignores: ["node_modules/**", "eslint.config.mjs"] },
  js.configs.recommended,
  tseslint.configs.recommendedTypeChecked,
  {
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
    // Code-execution rules recommended leaves off.
    rules: { "no-eval": "error", "no-new-func": "error" },
  },
);
EOF
cat >src/users.ts <<'EOF'
export function legacy(): string {
  const unused = 1;
  return "legacy";
}

export async function save(name: string): Promise<void> {
  await Promise.resolve(name);
}

export function trusted(): unknown {
  // eslint-disable-next-line no-eval
  return eval("1 + 1");
}
EOF
git add -A
git -c user.name=e2e -c user.email=e2e@example.invalid commit -q -m base

echo "== 2. a change that adds ESLint diagnostics"
cat >>src/users.ts <<'EOF'

export function run(expr: string): number {
  const debug = true;
  setTimeout("tick(" + expr + ")", 10);
  return Number(eval(expr));
}

export function build(body: string): () => void {
  return new Function(body) as () => void;
}

export function fire(name: string): void {
  save(name);
}

export function vetted(): unknown {
  // eslint-disable-next-line no-eval
  return eval("2 + 2");
}
EOF
git diff >"$work/change.patch"
echo "   $(grep -c '^+[^+]' "$work/change.patch") added line(s) in the diff"

echo "== 3. real ESLint SARIF ($image)"
"$container" image exists "$image" 2>/dev/null || "$container" pull -q "$image" >/dev/null
run() {
	"$container" run --rm -v "$repo:$repo:Z" -v "$npm_cache:/npm-cache:Z" -w "$repo" \
		-e npm_config_cache=/npm-cache -e npm_config_update_notifier=false "$image" "$@"
}
run npm install --no-audit --no-fund --loglevel=error >"$work/npm.log" 2>&1 ||
	{ tail -20 "$work/npm.log"; fail "npm install failed"; }
# ESLint exits 1 when it reports errors; 2 means it could not run.
set +e
run npx --no-install eslint -f @microsoft/eslint-formatter-sarif -o eslint.sarif . >"$work/eslint.log" 2>&1
code=$?
set -e
((code <= 1)) || { tail -20 "$work/eslint.log"; fail "eslint exited $code"; }
[[ -s $repo/eslint.sarif ]] || { tail -20 "$work/eslint.log"; fail "eslint wrote no SARIF"; }
echo "   node $(run node --version), eslint $(run npx --no-install eslint --version)"
python3 -I - "$repo/eslint.sarif" "$repo" <<'EOF'
import json, sys
doc = json.load(open(sys.argv[1]))
print(f'   SARIF {doc.get("version")}, {len(doc["runs"])} run(s)')
for run in doc["runs"]:
    drv = run["tool"]["driver"]
    print(f'   {drv["name"]} {drv.get("version", drv.get("semanticVersion", ""))}: {len(run.get("results", []))} result(s)')
    for r in run.get("results", []):
        loc = (r.get("locations") or [{}])[0].get("physicalLocation", {})
        uri = loc.get("artifactLocation", {}).get("uri", "?").replace("file://" + sys.argv[2] + "/", "")
        sup = " (suppressed)" if r.get("suppressions") else ""
        print(f'   {uri}:{loc.get("region", {}).get("startLine", "-")} {r.get("ruleId")}{sup}: {r["message"]["text"][:60]}')
EOF

if [[ -n "${SARIF_FIXTURE_OUT:-}" ]]; then
	sed "s#$repo#/work/repo#g" "$repo/eslint.sarif" >"$SARIF_FIXTURE_OUT"
	echo "   fixture written to $SARIF_FIXTURE_OUT (root rewritten to /work/repo)"
fi

echo "== 4. pr-review"
if [[ -n "${VIZZINI_LITELLM_URL:-}" && -n "${VIZZINI_MODEL:-}" ]]; then
	mode=real
	echo "   model: $VIZZINI_MODEL via $VIZZINI_LITELLM_URL"
else
	mode=stub
	addr="127.0.0.1:${STUB_PORT:-18771}"
	(cd "$root" && go build -o "$work/stubllm" ./scripts/e2e/stubllm)
	"$work/stubllm" -addr "$addr" -log "$work/prompts.log" 2>"$work/stubllm.err" &
	stub_pid=$!
	for _ in $(seq 50); do curl -s -o /dev/null "http://$addr/" && break; sleep 0.1; done
	export VIZZINI_LITELLM_URL="http://$addr/v1" VIZZINI_MODEL=stub
	echo "   model: stubllm (no VIZZINI_LITELLM_URL set)"
fi

set +e
"$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$repo/eslint.sarif" --format json >"$work/review.json" 2>"$work/stderr"
code=$?
"$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$repo/eslint.sarif" --fail-on none >"$work/review.md" 2>/dev/null
set -e
echo "   exit code $code; stderr: $(cat "$work/stderr")"

echo "== 5. assertions"
python3 -I "$root/scripts/e2e/assert_lint.py" "$repo/eslint.sarif" "$work/change.patch" "$work/review.json" "$mode" "$code" "$work/prompts.log" "$repo"

echo "== 6. Markdown review"
sed 's/^/   | /' "$work/review.md"
