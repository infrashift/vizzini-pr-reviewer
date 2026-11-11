---
id: "01a1ca11-ff98-760f-bc10-ff0f952fb965"
title: "ISSUE-014: Prove SARIF lint input with real ruff (Python) and Roslyn (C#) output"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "analysis"
  - "lint"
  - "sarif"
  - "python"
  - "csharp"
created: "2026-11-11T04:05:39-06:00"
timestamp: "2026-11-11T04:05:39-06:00"
description: "Capture real ruff and Roslyn SARIF as fixtures and add Python and C# end-to-end proofs mirroring make e2e-lint."
---

# ISSUE-014: Prove SARIF lint input with real ruff (Python) and Roslyn (C#) output

**Status:** open · **Phase:** 4 · **Split from:** [ISSUE-012](/issues/issue-012-multi-language-lint-input-via-sarif--01a1c9ed12c7.md), at the user's request (Python and C# as a follow-up)

## Description
ISSUE-012 shipped the SARIF parser, format auto-detection and repeatable `--lint-json`, tested against fixtures **hand-written to the SARIF 2.1.0 spec** (`internal/analysis/testdata/sarif/ruff-style.sarif`, `roslyn-style.sarif`). This issue replaces that modeling with captured tool output and proves the pipeline end to end, as `make e2e-lint` does for Go.

## Plan
- **Python (ruff):** ruff isn't installed, but `uvx ruff` works here (uv 0.5.11). Confirm the current flag (`ruff check --output-format=sarif`). Build a throwaway repo with a pre-existing issue and a change that adds new ones (e.g. S608 string-built SQL, F401 unused import), capture the SARIF, then run `pr-review --lint-json ruff.sarif` with the stub model.
- **C# (Roslyn):** `dotnet` isn't installed. Run the .NET SDK in podman (`mcr.microsoft.com/dotnet/sdk`) and capture SARIF via MSBuild's `ErrorLog` (`dotnet build -p:ErrorLog=roslyn.sarif%2Cversion=2.1`; confirm the syntax). Roslyn writes absolute `file://` URIs, which is the case `LintRoot` exists for. Paths inside the container differ from the host, so mount the repo at the same absolute path or pass a matching root.
- Generalize `scripts/e2e-golint.sh`, or add `e2e-pylint.sh` / `e2e-cslint.sh`, behind `make e2e-lint-python` / `e2e-lint-csharp`. Skip gracefully when uv or podman is missing.

## Acceptance Criteria
- [ ] Real ruff SARIF fixture committed (with the ruff version noted) and parsed in a unit test
- [ ] Real Roslyn SARIF fixture committed (SDK version noted) and parsed in a unit test, including absolute-URI relativization
- [ ] Python end-to-end proof: only changed-line diagnostics reach the prompt, findings anchored, the gate behaves
- [ ] C# end-to-end proof, same assertions
- [ ] Docs: Lint-Aware Reviews lists the verified commands and versions

## Related
- [ISSUE-012: Multi-language lint input via SARIF](/issues/issue-012-multi-language-lint-input-via-sarif--01a1c9ed12c7.md)
- [ISSUE-005: Hybrid verification](/issues/issue-005-hybrid-verification-static-analysis-plus-ai--01a1c9b6156a.md)
