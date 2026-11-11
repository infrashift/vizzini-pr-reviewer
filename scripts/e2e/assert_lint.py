"""Shared assertions for the lint-aware end-to-end proofs (e2e-*lint.sh).

Reads the linter report on its own terms, independently of Vizzini's Go
parsers, so the check is not circular:

  golangci-lint JSON   {"Issues": [{"FromLinter", "Text", "Pos": {...}}]}
  SARIF 2.1.0          {"runs": [{"results": [{"locations": [...]}]}]}

Usage: assert_lint.py REPORT PATCH REVIEW_JSON MODE EXIT_CODE PROMPTS_LOG ROOT
"""
import json
import re
import sys
from urllib.parse import unquote


def ok(msg):
    print(f"  \033[32mPASS\033[0m {msg}")


def die(msg):
    print(f"  \033[31mFAIL\033[0m {msg}")
    sys.exit(1)


def first_json(path):
    # golangci-lint v2 may append stats text after the JSON document.
    return json.JSONDecoder().raw_decode(open(path).read())[0]


def in_effect(result):
    """SARIF 2.1.0 3.35: a suppression with no status, or status "accepted",
    is in effect; "underReview" and "rejected" are not."""
    return any(x.get("status", "accepted") == "accepted" for x in result.get("suppressions") or [])


def issues(path, root, want_suppressed=False):
    """Return [(file, line, tool)] from either report format: the active
    diagnostics, or with want_suppressed the suppressed ones instead."""
    doc = first_json(path)
    if "Issues" in doc:
        return [] if want_suppressed else [(i["Pos"]["Filename"], i["Pos"]["Line"], i["FromLinter"]) for i in doc["Issues"] or []]
    out = []
    prefix = "file://" + root.rstrip("/") + "/"
    for run in doc["runs"]:
        tool = run["tool"]["driver"]["name"]
        for r in run.get("results", []):
            if not r.get("locations") or in_effect(r) != want_suppressed:
                continue
            loc = r["locations"][0]["physicalLocation"]
            uri = unquote(loc["artifactLocation"]["uri"])
            line = loc.get("region", {}).get("startLine", 0)
            if uri.startswith(prefix):
                uri = uri[len(prefix):]
            elif uri.startswith("file://"):
                continue  # outside the repository
            if line:
                out.append((uri, line, tool))
    return out


def added_lines(patch_path):
    """Map file -> set of new-side line numbers the diff adds."""
    added, cur, new = {}, None, 0
    for line in open(patch_path):
        if line.startswith("+++ b/"):
            cur = line[6:].strip()
            added.setdefault(cur, set())
            continue
        if line.startswith("+++") or line.startswith("---"):
            continue
        m = re.match(r"@@ -\d+(?:,\d+)? \+(\d+)", line)
        if m:
            new = int(m.group(1))
            continue
        if line.startswith("+"):
            added[cur].add(new)
            new += 1
        elif line.startswith(" "):
            new += 1
    return added


def main():
    report, patch, review_path, mode, code, prompts_path, root = sys.argv[1:]
    code = int(code)

    added = added_lines(patch)
    all_issues = issues(report, root)
    on_added = [i for i in all_issues if i[1] in added.get(i[0], set())]
    untouched = [i for i in all_issues if i[1] not in added.get(i[0], set())]
    if not on_added:
        die("the linter reported nothing on added lines; the fixture is broken")
    if not untouched:
        die("expected a pre-existing issue on an untouched line")
    ok(f"linter: {len(on_added)} issue(s) on added lines, {len(untouched)} on untouched lines")

    review = json.load(open(review_path))
    if review.get("apiVersion") != "vizzini.infrashift.io/v1" or review.get("kind") != "Review":
        die("review JSON lacks the report envelope")
    if review.get("diagnostics") != len(on_added):
        die(f'diagnostics={review.get("diagnostics")}, want {len(on_added)}')
    ok(f"pr-review injected exactly the {len(on_added)} changed-line diagnostic(s)")

    for f in review["findings"]:
        if f["file"] not in added:
            die(f"finding outside the diff: {f}")
        if mode == "stub" and not any(f["start_line"] <= ln <= f["end_line"] for ln in added[f["file"]]):
            die(f"stub finding not on an added line: {f}")
    if review["chunks"] < 1:
        die("nothing was reviewed")
    ok(f'{len(review["findings"])} finding(s), all schema-validated and anchored to the diff')

    if mode == "stub":
        block = open(prompts_path).read().split("<lint>")[-1].split("</lint>")[0]
        for f, ln, _ in on_added:
            if f"{f}:{ln}" not in block:
                die(f"prompt lacks changed-line diagnostic {f}:{ln}")
        for f, ln, _ in untouched:
            if re.search(rf"^{re.escape(f)}:{ln}[: ]", block, re.M):
                die(f"prompt <lint> block leaked untouched-line diagnostic {f}:{ln}")
        ok("prompt <lint> block holds the new issues and none of the pre-existing ones")
        quiet = [i for i in issues(report, root, want_suppressed=True) if i[1] in added.get(i[0], set())]
        for f, ln, _ in quiet:
            if re.search(rf"^{re.escape(f)}:{ln}[: ]", block, re.M):
                die(f"prompt <lint> block leaked suppressed diagnostic {f}:{ln}")
        if quiet:
            ok(f"{len(quiet)} suppressed diagnostic(s) on added lines stayed out of the prompt")
        if len(review["findings"]) != len(on_added):
            die("stub should echo one finding per diagnostic")

    # The default gate blocks Critical and Major, whatever produced them.
    blocking = any(f["severity"] in ("Critical", "Major") for f in review["findings"])
    want = 2 if blocking else 0
    if mode == "real" and code not in (0, 2):
        die(f"exit code {code}")
    if mode == "stub" and code != want:
        die(f"exit code {code}, want {want}")
    ok(f"policy gate: exit {code} ({'blocking finding' if blocking else 'nothing blocking'})")

    if review["auto_approve"] and blocking:
        die("auto_approve with a blocking finding")
    ok(f'risk {review["risk_score"]} (diff {review["base_risk_score"]}, model {review["model_risk_score"]}), auto_approve {str(review["auto_approve"]).lower()}')


if __name__ == "__main__":
    main()
