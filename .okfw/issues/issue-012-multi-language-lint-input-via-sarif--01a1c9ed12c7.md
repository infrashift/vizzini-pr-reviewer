---
id: "01a1c9ed-12c7-7cce-9d55-6cc0dba6720c"
title: "ISSUE-012: Multi-language lint input via SARIF"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "analysis"
  - "lint"
  - "sarif"
  - "closed"
created: "2026-11-11T03:25:19-06:00"
timestamp: "2026-11-11T04:05:39-06:00"
description: "Accept SARIF reports (ruff, Roslyn analyzers, Semgrep, CodeQL) alongside golangci-lint JSON so lint-aware reviews work beyond Go."
---

# ISSUE-012: Multi-language lint input via SARIF

**Status:** closed (real-tool proofs split to ISSUE-014) · **Phase:** 4 (outside the current milestones) · **Raised:** 2026-11-11 by the user, after the Go linting proof in [ISSUE-005](/issues/issue-005-hybrid-verification-static-analysis-plus-ai--01a1c9b6156a.md)

## Description
Lint-aware reviews (Milestone 4.2) only read golangci-lint's JSON report, so only Go projects benefit. Most of the pipeline is already language-agnostic:

- **Language-agnostic today:** diff parsing, chunking, prompts, the CUE schema and finding anchoring work for any text diff, `.py` and `.cs` included. `analysis.OnChangedLines` and the `<lint>` prompt block use only file, line and message, so any tool's diagnostics fit once parsed into an `analysis.Diagnostic`.
- **Go-only today:**
  - Linter input: golangci-lint JSON only.
  - Risk complexity (4.4): decision points are counted with Go's scanner, so other languages score churn × path weight only.
  - Dependency walkthroughs (4.3): only `.go` files are ordered, and the rest follow in diff order.

## Proposal: one input format, many linters
Add a SARIF parser next to `ParseGolangciJSON`, instead of one parser per tool. SARIF (Static Analysis Results Interchange Format) is the standard interchange format and plain JSON, so no new dependency is needed (ADR-0005, pinned list).

| Language | Tool | How (confirm current flags when building) |
|---|---|---|
| Python | ruff | `ruff check --output-format=sarif` |
| C# / .NET | Roslyn analyzers | MSBuild `ErrorLog` property writes SARIF during `dotnet build` |
| Many | Semgrep, CodeQL, … | native SARIF output |

- **CLI:** add `--lint-sarif FILE`, or sniff the format behind `--lint-json` (a SARIF document has `"version"` and `"runs"` at the top level, while golangci-lint has `"Issues"`). Possibly accept several reports at once.
- **Paths:** SARIF locations are URIs (`artifactLocation.uri`), sometimes relative to a `uriBaseId` resolved through `originalUriBaseIds`. Normalize them to repo-relative slash paths. This is the same "run from the repo root" contract golangci-lint has, plus `file://` stripping.
- **Mapping:** `ruleId` (and `tool.driver.name`) becomes `Linter`, `message.text` becomes `Text`, and `region.startLine`/`startColumn` become `Line`/`Column`. SARIF `level` (error/warning/note) could set a severity hint for the model.

## Acceptance Criteria
- [x] `analysis.ParseSARIF(io.Reader)` covering multiple runs, `uriBaseId` resolution, `file://` URIs and missing regions (skipped)
- [x] CLI and `ReviewOptions` accept SARIF (flag or format sniffing), with a documented path contract
- [ ] Fixtures captured from real ruff and Roslyn (`dotnet build`) SARIF output → moved to [ISSUE-014](/issues/issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md)
- [ ] An `e2e-lint`-style proof for Python (ruff) mirroring `scripts/e2e-golint.sh` → moved to [ISSUE-014](/issues/issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md)
- [x] Docs: Lint-Aware Reviews guide covers SARIF tools

## Out of scope (separate, larger work)
- **Complexity scoring and dependency ordering for Python or C#.** There is no pure-Go Python AST in the standard library, and cgo parsers like tree-sitter are ruled out by [ADR-0002](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md). Token-level heuristics like the 4.3 Go fallback are the realistic pure-Go route. File a separate issue if wanted.

## Resolution
Done 2026-11-11, except the real-tool proofs, which the user moved to [ISSUE-014](/issues/issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md).
- **`internal/analysis/sarif.go`:**
  - `ParseSARIF(r, root)` reads SARIF 2.1.0: every run, and each result's first physical location. `Linter` is the tool driver's name and `Text` is `ruleId: message`; when there is no message text it falls back to the rule's description (by `ruleIndex` or id).
  - Skips results of kind `pass`/`notApplicable`, suppressions in effect (no status, or `accepted`; `underReview`/`rejected` are kept), and results without a location or `startLine`.
  - URIs: `uriBaseId` chains resolve through `originalUriBaseIds` (an unknown base means the repo root, and a cyclic chain is rejected, a bug the tests caught). `file://` URIs, including a host form, become root-relative. Percent-encoding is decoded and Windows drive paths are handled. Absolute locations outside the root, and non-file schemes, are dropped.
- **Auto-detection:** `ParseLintReport(r, root)` sniffs the first JSON value: `"runs"` means SARIF and `"Issues"` means golangci-lint (still tolerating v2's trailing stats text). Absolute golangci paths are also relativized.
- **API:** `ReviewOptions.LintReport io.Reader` is replaced by `LintReports []io.Reader` plus `LintRoot string`. This was done pre-release with no consumers, so no deprecation shim. Errors name the report (`lint report 2: …`).
- **CLI:** `--lint-json` is repeatable and accepts either format. `-` is rejected in any position. The working directory is the `LintRoot`, under the same "run from the repo root" contract as before.
- **Tests:** ruff-style and Roslyn-style fixtures, including a second run from semgrep, a URI resolution table (12 cases), format detection, SARIF through the changed-line filter, a polyglot engine review (golangci-lint + SARIF, with `file://` and `uriBaseId` locations, and an off-diff result dropped), naming of a bad second report, and the repeatable CLI flag. `make e2e-lint` and `make e2e-garmr` still pass.
- **Not changed:** risk complexity (4.4) and dependency ordering (4.3) are still Go-only, as scoped.

## Related
- [ISSUE-005: Hybrid verification](/issues/issue-005-hybrid-verification-static-analysis-plus-ai--01a1c9b6156a.md)
- [ISSUE-007: Automated risk scoring](/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)
- [ISSUE-006: Semantic dependency walkthroughs](/issues/issue-006-semantic-dependency-walkthroughs--01a1c9b61571.md)
