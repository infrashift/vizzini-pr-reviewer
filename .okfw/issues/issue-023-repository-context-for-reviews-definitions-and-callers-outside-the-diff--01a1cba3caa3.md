---
id: "01a1cba3-caa3-77d9-8efc-e74549bc6fea"
title: "ISSUE-023: Repository context for reviews (definitions and callers outside the diff)"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "review"
  - "context"
  - "quality"
  - "open"
created: "2026-11-11T11:24:31-06:00"
timestamp: "2026-11-11T11:24:31-06:00"
description: "Give the model the definitions of symbols the diff uses and the callers of symbols it changes, read from the repository at the PR head within a token budget, so reviews can reason beyond the diff."
---

# ISSUE-023: Repository context for reviews (definitions and callers outside the diff)

**Status:** open · **Raised:** 2026-11-11 · **Touches:** [ADR-0007](/decisions/adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md) (anchoring), [ISSUE-008](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md) (knowledge grounding)

## Problem
Vizzini sees only the diff, plus lint output and knowledge documents. A model that sees `orders.FindByCustomerAsync(customer, ct)` can't check what that method does, and a change to a function's contract can't be checked against its callers. Many real defects live in exactly that gap. PR-Agent (dynamic/RAG context) and Kodus (AST) both go beyond the diff.

## Design
- **Input:** `ReviewOptions.Repo fs.FS`, the repository at the PR head, plus a CLI flag `--repo DIR`. It is optional: without it, behaviour is unchanged and Vizzini stays usable on a bare patch.
- **Symbols** come from each chunk's changed lines:
  - **used:** called or instantiated, e.g. `name(`, `new Name(`, `x.name(`;
  - **defined:** a function, method or type declared on a changed line.

  Keywords, very short names and a stop list (`String`, `len`, `get`, …) are dropped.
- **Retrieval:** one deterministic pass over the repository's source files of the same language family. It skips `.git`, `vendor`, `node_modules`, generated or excluded paths and oversized or binary files, and is capped in file count and bytes. It finds:
  - **definitions** of used symbols, through language-family declaration patterns (Go, C-family, JS/TS, Python, Ruby), as a snippet from the declaration line onward;
  - **call sites** of changed definitions, as ±2 lines.

  Regions already shown in the chunk are excluded.
- **Budget:** `Config.MaxContextTokens` (default 1500 per chunk). Definitions come before callers, nearer files (same directory, then same top-level) before farther ones, and ties break by path and line.
- **Prompt:** a `<context>` block of read-only snippets labelled `path:start-end`. The model is told to use them to judge the change, and that findings must still be on diff lines; anchoring (ADR-0007) is unchanged.
- **Report:** `context_snippets` counts the snippets given (closed `#ReviewReport`).
- **Pure Go, no new dependency:** scanning plus regexps, not a parser. Precision is "good enough to help", not a language server.

## Acceptance criteria
- [ ] Symbol extraction tests per family (used vs defined, keywords and stop list dropped)
- [ ] Definition and caller retrieval tests on a small multi-language fixture repo, including exclusion of shown regions, skipped directories and determinism
- [ ] Budget and ordering tests, and an engine test that the `<context>` block reaches the prompt and findings stay anchored to the diff
- [ ] CLI `--repo`, docs, an ADR
- [ ] vizzini-uat passes `--repo`; the C# fixture's controller sees `OrderRepository.FindByCustomerAsync`
