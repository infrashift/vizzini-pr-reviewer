---
id: "01a1c9b6-1571-7bb5-9b53-5231d88fab98"
title: "ISSUE-006: Semantic dependency walkthroughs"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "phase-4"
  - "analysis"
  - "milestone-4-3"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Order changed Go files by an AST-derived dependency DAG (Kahn's algorithm) for mrman's file tree."
---

# ISSUE-006: Semantic dependency walkthroughs

**Status:** open · **Phase:** 4 · **Milestone:** 4.3 · **PLAN.md:** Milestone 4.3: Semantic Dependency Walkthroughs

## Description
Order PR diffs by architectural dependency (models → logic → UI) for review in the `mrman` TUI.

## Acceptance Criteria
- [ ] Parse modified `.go` files with `go/parser` / `go/ast` (imports, struct/interface decls)
- [ ] Build a DAG; Kahn's topological sort with deterministic cycle-breaking by path depth
- [ ] Expose `OrderFilesByDependency(files []*gitdiff.File) []*gitdiff.File`
- [ ] Test: `ui.go` → `service.go` → `model.go` orders `model.go` first

## Related
- [ADR-0002: Pure Go with CGO_ENABLED=0](/decisions/adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md)
- [ADR-0005: Stateless library-first API with minimal abstractions](/decisions/adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md)
- Depends on: [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](/issues/issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md)
