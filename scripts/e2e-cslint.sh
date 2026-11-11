#!/usr/bin/env bash
# End-to-end proof of lint-aware C# review via Roslyn SARIF (ISSUE-014):
#
#   1. a throwaway C# repo whose base commit has a PRE-EXISTING warning
#   2. a change that adds NEW ones (CA2100 SQL built from input, CS0168)
#   3. real Roslyn SARIF from `dotnet build` (MSBuild ErrorLog) in the .NET
#      SDK container; the repo is mounted at the same absolute path so the
#      absolute file:// URIs Roslyn writes resolve on the host
#   4. the real `git diff` and bin/pr-review, run from the repo root
#
# Needs podman (or docker via $CONTAINER) and the SDK image ($DOTNET_IMAGE,
# default mcr.microsoft.com/dotnet/sdk:10.0, pulled if missing); without a
# container runtime the script prints SKIP and exits 0. The model is your
# LiteLLM proxy if VIZZINI_LITELLM_URL/VIZZINI_MODEL are set, else stubllm.
#
# SARIF_FIXTURE_OUT=path also saves the captured SARIF as a unit-test
# fixture, with the temp repo path rewritten to /work/repo.
#
# Usage: scripts/e2e-cslint.sh   (or: make e2e-lint-csharp)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prreview="$root/bin/pr-review"
export TIKTOKEN_CACHE_DIR="${TIKTOKEN_CACHE_DIR:-$root/.cache/tiktoken}"
container="${CONTAINER:-podman}"
image="${DOTNET_IMAGE:-mcr.microsoft.com/dotnet/sdk:10.0}"

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
mkdir -p "$repo/src"
cd "$repo"
git init -q -b main
printf 'bin/\nobj/\nroslyn.sarif\n' >.gitignore
cat >App.csproj <<'EOF'
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net10.0</TargetFramework>
    <Nullable>enable</Nullable>
    <!-- CA2100 and the other security rules are off by default. -->
    <AnalysisLevelSecurity>latest-all</AnalysisLevelSecurity>
    <ErrorLog>roslyn.sarif,version=2.1</ErrorLog>
  </PropertyGroup>
</Project>
EOF
cat >src/Users.cs <<'EOF'
using System.Data.Common;

namespace App;

public static class Users
{
    public static void Legacy()
    {
        int unused;
    }

    public static object? Find(DbCommand cmd, string name)
    {
        cmd.CommandText = "SELECT id FROM users WHERE name = @name";
        var p = cmd.CreateParameter();
        p.ParameterName = "@name";
        p.Value = name;
        cmd.Parameters.Add(p);
        return cmd.ExecuteScalar();
    }
}
EOF
git add -A
git -c user.name=e2e -c user.email=e2e@example.invalid commit -q -m base

echo "== 2. a change that adds Roslyn warnings"
cat >src/Users.cs <<'EOF'
using System.Data.Common;

namespace App;

public static class Users
{
    public static void Legacy()
    {
        int unused;
    }

    public static object? Find(DbCommand cmd, string name)
    {
        int retries;
        cmd.CommandText = "SELECT id FROM users WHERE name = '" + name + "'";
        return cmd.ExecuteScalar();
    }
}
EOF
git diff >"$work/change.patch"
echo "   $(grep -c '^+[^+]' "$work/change.patch") added line(s) in the diff"

echo "== 3. real Roslyn SARIF (dotnet build in $image)"
"$container" image exists "$image" 2>/dev/null || "$container" pull -q "$image" >/dev/null
"$container" run --rm -v "$repo:$repo:Z" -w "$repo" "$image" dotnet build -nologo >"$work/build.log" 2>&1 ||
	{ tail -20 "$work/build.log"; fail "dotnet build failed"; }
echo "   $("$container" run --rm "$image" dotnet --version | sed 's/^/.NET SDK /')"
python3 -I - "$repo/roslyn.sarif" "$repo" <<'EOF'
import json, sys
run = json.load(open(sys.argv[1]))["runs"][0]
print(f'   {run["tool"]["driver"]["name"]} {run["tool"]["driver"].get("semanticVersion", "")}')
for r in run["results"]:
    loc = r["locations"][0]["physicalLocation"]
    print(f'   {loc["artifactLocation"]["uri"].replace("file://" + sys.argv[2] + "/", "")}:{loc["region"]["startLine"]} {r["ruleId"]}: {r["message"]["text"][:70]}')
EOF

if [[ -n "${SARIF_FIXTURE_OUT:-}" ]]; then
	sed "s#$repo#/work/repo#g" "$repo/roslyn.sarif" >"$SARIF_FIXTURE_OUT"
	echo "   fixture written to $SARIF_FIXTURE_OUT (root rewritten to /work/repo)"
fi

echo "== 4. pr-review"
if [[ -n "${VIZZINI_LITELLM_URL:-}" && -n "${VIZZINI_MODEL:-}" ]]; then
	mode=real
	echo "   model: $VIZZINI_MODEL via $VIZZINI_LITELLM_URL"
else
	mode=stub
	addr="127.0.0.1:${STUB_PORT:-18770}"
	(cd "$root" && go build -o "$work/stubllm" ./scripts/e2e/stubllm)
	"$work/stubllm" -addr "$addr" -log "$work/prompts.log" 2>"$work/stubllm.err" &
	stub_pid=$!
	for _ in $(seq 50); do curl -s -o /dev/null "http://$addr/" && break; sleep 0.1; done
	export VIZZINI_LITELLM_URL="http://$addr/v1" VIZZINI_MODEL=stub
	echo "   model: stubllm (no VIZZINI_LITELLM_URL set)"
fi

set +e
"$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$repo/roslyn.sarif" --format json >"$work/review.json" 2>"$work/stderr"
code=$?
"$prreview" --mode review --patch-file "$work/change.patch" --lint-json "$repo/roslyn.sarif" --fail-on none >"$work/review.md" 2>/dev/null
set -e
echo "   exit code $code; stderr: $(cat "$work/stderr")"

echo "== 5. assertions"
python3 -I "$root/scripts/e2e/assert_lint.py" "$repo/roslyn.sarif" "$work/change.patch" "$work/review.json" "$mode" "$code" "$work/prompts.log" "$repo"

echo "== 6. Markdown review"
sed 's/^/   | /' "$work/review.md"
