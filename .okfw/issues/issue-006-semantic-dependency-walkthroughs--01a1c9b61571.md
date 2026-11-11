---
id: "01a1c9b6-1571-7bb5-9b53-5231d88fab98"
title: "ISSUE-006: Semantic dependency walkthroughs"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "phase-4"
  - "analysis"
  - "milestone-4-3"
  - "closed"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T03:17:00-06:00"
description: "Order changed Go files by an AST-derived dependency DAG (Kahn's algorithm) for mrman's file tree."
---

# ISSUE-006: Semantic dependency walkthroughs

**Status:** closed · **Phase:** 4 · **Milestone:** 4.3 · **PLAN.md:** Milestone 4.3: Semantic Dependency Walkthroughs

## Description
Order PR diffs by architectural dependency (models → logic → UI) for review in the `mrman` TUI.

## Acceptance Criteria
- [x] Parse modified `.go` files with `go/parser` / `go/ast` (imports, struct/interface decls)
- [x] Build a DAG; Kahn's topological sort with deterministic cycle-breaking by path depth
- [x] Expose `OrderFilesByDependency(files []*gitdiff.File) []*gitdiff.File`
- [x] Test: `ui.go` → `service.go` → `model.go` orders `model.go` first

## Resolution
Done 2026-11-11.
- **Diff-only by design:** the input is just the diff, so no checkout or filesystem is needed, which keeps the engine stateless. New files are complete. For modified files, the new side of the hunks (context plus added lines) is reassembled. If that parses with `go/parser` (`SkipObjectResolution`), imports, `TypeSpec` declarations (struct, interface and every other type) and identifier references come from `go/ast`. Hunk fragments usually don't parse, so a `go/scanner` fallback extracts the same three sets from tokens: imports including blocks, `type X` and `type ( … )` blocks with paren depth tracking, and other identifiers as references.
- **Edges:** A depends on B if A imports B's package or if they share a directory and A references a type B declares. The module path is unknown from a diff, so import paths match directories on whole trailing segments (`example.com/app/internal/model` ↔ `internal/model`, but not `supermodel`).
- **Order:** Kahn's algorithm repeatedly places a file whose dependencies are all placed. When several are ready, or none is (a cycle, which Go allows within a package), the choice is made by path depth (shallower first), then path. Non-Go files follow in diff order. Deleted Go files have no edges and order by path.
- **API:** `analysis.OrderFilesByDependency` is re-exported as `reviewer.OrderFilesByDependency(files []*gitdiff.File) []*gitdiff.File` for mrman's file tree. The input slice is not modified.
- **Tests:** the 3-layer acceptance test (ui → service → model gives model first), determinism across input permutations, same-package type references, a modified-file fragment going through the token fallback including a `type (…)` block with func-type params, a token extractor unit test, cycles broken at the same depth and across depths, non-Go and deleted files, no input mutation, and import/directory matching.
- **Not done (deliberately):** Review still chunks files in diff order. Ordering review chunks by dependency could help the model, but it wasn't asked for.

## Related
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
