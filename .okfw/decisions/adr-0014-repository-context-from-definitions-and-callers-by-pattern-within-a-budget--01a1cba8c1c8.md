---
id: "01a1cba8-c1c8-751c-a15f-ae497eb3911b"
title: "ADR-0014: Repository context from definitions and callers, by pattern, within a budget"
type: "ADR"
tags:
  - "adr"
  - "review"
  - "context"
created: "2026-11-11T11:29:56-06:00"
timestamp: "2026-11-11T11:29:56-06:00"
description: "With a repository given, each chunk's prompt gains the definitions of symbols it uses and the callers of symbols it changes, found by language-family patterns in one deterministic scan, within MaxContextTokens; findings stay anchored to the diff."
---

# ADR-0014: Repository context from definitions and callers, by pattern, within a budget

**Status:** Accepted
**Date:** 2026-11-11
**Source:** [ISSUE-023](/issues/issue-023-repository-context-for-reviews-definitions-and-callers-outside-the-diff--01a1cba3caa3.md)
**Related:** [ADR-0007](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md) (anchoring), [ADR-0009](/decisions/adr-0009-architecture-findings-must-cite-provided-knowledge-documents--01a1c9f25bc8.md) (citations)

## Context
Vizzini reviewed the diff alone. Defects that depend on code outside it are invisible: a callee that already validates, a caller a signature change breaks. Open-source alternatives add repository context through retrieval (RAG) or code-structure (AST) analysis.

## Decision
- **Input:** `ReviewOptions.Repo fs.FS` (CLI `--repo DIR`). It is optional, and without it nothing changes.
- **Symbols:** from changed lines per language family (Go, C-family, JS/TS, Python, Ruby, Rust).
  - **Used:** calls and `new X` on added lines.
  - **Defined:** declarations on added or deleted lines.

  Keywords, very common calls, names under three characters and names the chunk defines are dropped.
- **Retrieval:** `internal/repoctx` makes one `fs.WalkDir` pass, in lexical and therefore deterministic order, over files of the wanted families. It skips VCS, vendor, build and dot directories, the review's exclude globs, binary files and files over 512 KiB, and caps the scan at 20,000 files and 64 MiB. It returns:
  - **definitions:** the declaration to the end of its body, by brace depth or indentation, at most 30 lines;
  - **call sites:** ±2 lines.
- **Selection per chunk:**
  - never a region the chunk shows;
  - definitions before callers;
  - nearest files first (same directory, then same top level);
  - at most 2 definitions and 3 callers per symbol;
  - within `Config.MaxContextTokens` (default 1500).
- **Prompt:** a `<context>` block of numbered snippets. Findings must still be on diff lines, and anchoring (ADR-0007) is unchanged, so a finding on a context line goes back for repair.
- **Report:** `context_snippets` (closed `#ReviewReport`).

## Consequences
- Reviews can catch contract and caller breakage. This is measured by ISSUE-022's benchmark, not assumed.
- Name matching is approximate: overloads or common names can bring back unrelated code. The budget, the caps and the stop list bound the cost.
- **No new dependency, pure Go.**
- **Alternatives rejected:**
  - Language servers or tree-sitter: cgo or heavy dependencies, and per-language setup.
  - Embedding-based retrieval: needs an index and an embedding model, stateful, against "stateless library".
