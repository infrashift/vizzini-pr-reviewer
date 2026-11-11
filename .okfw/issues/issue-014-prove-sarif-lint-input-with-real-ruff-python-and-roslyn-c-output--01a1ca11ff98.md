---
id: "01a1ca11-ff98-760f-bc10-ff0f952fb965"
title: "ISSUE-014: Prove SARIF lint input with real ruff (Python) and Roslyn (C#) output"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "analysis"
  - "lint"
  - "sarif"
  - "python"
  - "csharp"
  - "closed"
created: "2026-11-11T04:05:39-06:00"
timestamp: "2026-11-11T04:23:49-06:00"
description: "Capture real ruff and Roslyn SARIF as fixtures and add Python and C# end-to-end proofs mirroring make e2e-lint."
---

# ISSUE-014: Prove SARIF lint input with real ruff (Python) and Roslyn (C#) output

**Status:** closed · **Phase:** 4 · **Split from:** [ISSUE-012](/issues/issue-012-multi-language-lint-input-via-sarif--01a1c9ed12c7.md), at the user's request (Python and C# as a follow-up)

## Description
ISSUE-012 shipped the SARIF parser, format auto-detection and repeatable `--lint-json`, tested against fixtures **hand-written to the SARIF 2.1.0 spec** (`internal/analysis/testdata/sarif/ruff-style.sarif`, `roslyn-style.sarif`). This issue replaces that modeling with captured tool output and proves the pipeline end to end, as `make e2e-lint` does for Go.

## Plan
- **Python (ruff):** ruff isn't installed, but `uvx ruff` works here (uv 0.5.11). Confirm the current flag (`ruff check --output-format=sarif`). Build a throwaway repo with a pre-existing issue and a change that adds new ones (e.g. S608 string-built SQL, F401 unused import), capture the SARIF, then run `pr-review --lint-json ruff.sarif` with the stub model.
- **C# (Roslyn):** `dotnet` isn't installed. Run the .NET SDK in podman (`mcr.microsoft.com/dotnet/sdk`) and capture SARIF via MSBuild's `ErrorLog` (`dotnet build -p:ErrorLog=roslyn.sarif%2Cversion=2.1`; confirm the syntax). Roslyn writes absolute `file://` URIs, which is the case `LintRoot` exists for. Paths inside the container differ from the host, so mount the repo at the same absolute path or pass a matching root.
- Generalize `scripts/e2e-golint.sh`, or add `e2e-pylint.sh` / `e2e-cslint.sh`, behind `make e2e-lint-python` / `e2e-lint-csharp`. Skip gracefully when uv or podman is missing.

## Acceptance Criteria
- [x] Real ruff SARIF fixture committed (with the ruff version noted) and parsed in a unit test
- [x] Real Roslyn SARIF fixture committed (SDK version noted) and parsed in a unit test, including absolute-URI relativization
- [x] Python end-to-end proof: only changed-line diagnostics reach the prompt, findings anchored, the gate behaves
- [x] C# end-to-end proof, same assertions
- [x] Docs: Lint-Aware Reviews lists the verified commands and versions

## Resolution
Done 2026-11-11.
- **Python (ruff 0.17.4, via `uvx ruff`):** `scripts/e2e-pylint.sh` (`make e2e-lint-python`). The base commit has F401 (`json`) and S307 (`eval`); the change adds F401 (`os`) and S608 (string-built SQL). Result: 4 real diagnostics, and only the 2 on added lines reached the prompt. S608 became a Major finding and the gate exited 2.
- **C# (Roslyn, C# compiler 5.9.0, .NET SDK 10.0.404 in `mcr.microsoft.com/dotnet/sdk:10.0` under podman):** `scripts/e2e-cslint.sh` (`make e2e-lint-csharp`). The base has CS0168; the change adds CS0168 and CA2100. Result: 3 real diagnostics, and only the 2 added reached the prompt; CA2100 Major, exit 2. The repo is mounted at the same absolute path so the URIs resolve.
- **Fixtures:** `internal/analysis/testdata/sarif/ruff-0.17.4.sarif` and `roslyn-5.9.0.sarif` were captured by the scripts' `SARIF_FIXTURE_OUT`, with only the temp repo path rewritten to `/work/repo` (no local paths; checked). Tested by `TestParseSARIF_RealRuff` and `TestParseSARIF_RealRoslyn`, including that the wrong root drops everything. The Roslyn fixture is 356 KB (331 rule descriptors) and is kept unpruned, because results use `ruleIndex`. The hand-written spec fixtures stay: they cover cases neither tool emitted (uriBaseId chains, suppressions, multiple runs).
- **Shared assertions:** `scripts/e2e/assert_lint.py` reads golangci JSON or SARIF independently of Vizzini's Go parsers. The Go proof now uses it too, with unchanged results.

Learned from real output:
- **Both ruff and Roslyn write absolute `file://` URIs.** My hand-written ruff fixture had assumed relative ones, so `LintRoot` matters for every SARIF tool tried.
- **Roslyn security analyzers are off by default.** `AnalysisModeSecurity` didn't enable them; `<AnalysisLevelSecurity>latest-all</AnalysisLevelSecurity>` did. The rule's `defaultConfiguration` in SARIF still says disabled, and the effective level is in `invocations[0].ruleConfigurationOverrides`.
- **CA2100 fires on `DbCommand.CommandText` but not through `IDbCommand`.**
- **Roslyn's driver name has spaces** ("Microsoft (R) Visual C# Compiler"). Vizzini handles it, but the stub's line regex didn't until it was fixed.
- **The stub model now treats security rule IDs from any tool as Major** (gosec, ruff S-rules, CA2100/CA3xxx/CA5xxx). The gate assertion checks the findings' severities rather than the tool.

## Related
- [ISSUE-012: Multi-language lint input via SARIF](/issues/issue-012-multi-language-lint-input-via-sarif--01a1c9ed12c7.md)
- [ISSUE-005: Hybrid verification](/issues/issue-005-hybrid-verification-static-analysis-plus-ai--01a1c9b6156a.md)
