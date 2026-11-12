---
id: "01a1cb5c-5aeb-7c7d-ba7d-76a418c0d6a8"
title: "ISSUE-021: Knowledge grounding by namespace beyond Go"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "knowledge"
  - "okf"
  - "analysis"
  - "closed"
created: "2026-11-11T10:06:29-06:00"
timestamp: "2026-11-11T10:06:29-06:00"
description: "Knowledge documents' domain and tags match only Go package clauses and directory names; read C#, Java/Kotlin/Scala, PHP, C++ and TypeScript namespace/package declarations too, and let qualified domains match nested namespaces."
---

# ISSUE-021: Knowledge grounding by namespace beyond Go

**Status:** closed · **Raised:** 2026-11-11 from the C# and TypeScript UATs ([ISSUE-016](/issues/issue-016-cross-tool-uat-on-c-with-live-claude-sonnet-5-5-and-opus-5-5--01a1caee2585.md), [ISSUE-018](/issues/issue-018-prove-sarif-lint-input-with-real-eslint-typescript-javascript-output--01a1cb1c9e84.md)) · **Touches:** [ISSUE-008](/issues/issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md) (OKF grounding)

## Problem
`okf.TargetsFor` reads a changed file's package from a Go-shaped `^\s*package\s+ident` line in its hunks. A `domain` or `tags` term then matches that name or a directory name. Consequences:
- **C#** `namespace Shop.Repositories;`, **PHP** `namespace App\Http;`, **C++** `namespace a::b {` and **TypeScript** `namespace Billing {` are never read. These documents can only match by directory, or by `x.paths` globs, which both UAT fixtures had to use.
- **Java, Kotlin and Scala are actively wrong.** `package com.acme.store;` matches the Go regex as `com`. A document with `domain: store` misses the file, while `domain: com` would match every `com.*` file.

## Design
- `Target.Package` becomes the file's declared package or namespace for any supported language, normalized to dots:

| language | declaration read | example → Package |
|---|---|---|
| Go | `package x` | `store` |
| Java, Kotlin, Scala, Groovy | `package a.b.c` (`;` optional) | `com.acme.store` |
| C# | `namespace A.B` (file-scoped or block) | `Shop.Repositories` |
| PHP | `namespace A\B;` | `App.Http` |
| C++ | `namespace a::b {` (also `inline namespace`) | `a.b` |
| TypeScript | `[export] [declare] namespace\|module A.B {` | `Billing` |

- **Matching** (domain and tags, case-insensitive): a single-segment term matches any segment of the package or any directory, as today. A qualified term (`Shop.Repositories`, or written with `::`, a backslash or `/`) matches that namespace or any namespace nested under it.
- **Unchanged:** Python, Ruby, Rust, Swift and plain JS have no file-level namespace declaration, so their modules are their paths and directory matching covers them.
- **Limitation, unchanged:** Vizzini sees only the diff. The declaration counts only if it is in a hunk: on an added or context line, which includes every new file.

## Acceptance criteria
- [x] `TargetsFor` table test: every row above, plus a Go file unchanged, Java no longer collapsing to `com`, the first declaration winning, and no declaration giving an empty Package
- [x] `Match` tests: a C# namespace matches a single-segment domain; a qualified domain matches its namespace and nested ones but not siblings (`Shop.Repo` vs `Shop.Repositories`); `::`, backslash and `/` spellings of a domain work; tags follow the same rules at tag priority
- [x] Existing grounding tests unchanged and passing
- [x] Knowledge guide: the domain row and a per-language table
- [x] vizzini-lab: the C# fixture's data-access ADR can use `domain: Shop.Repositories`-style grounding (stub run confirms the knowledge docs are selected)

## Resolution (2026-11-11)
Implemented in `internal/okf/grounder.go` (Vizzini `ffe8e03`). `TargetsFor` reads the first package or namespace declaration on a surviving hunk line for each language in the table above and normalizes it to dots. `namesAny` matches a single-name term against any namespace segment or directory, and a qualified term against that namespace, nested ones, or the same run of directories.

**Tests:**
- `TestTargetsFor_Namespaces`: 14 files across 9 languages, including Java no longer collapsing to `com`, the first declaration winning, commented and deleted lines ignored, and `declare module "x"` ignored.
- `TestMatch_Namespaces`: segment, qualified, nested, sibling, parent and separator-spelling cases.
- The existing grounding tests are unchanged.

**UAT:**
- The C# fixture's ADRs now use `x.domain` (`Shop`, and `Shop.Services` / `Shop.Controllers`) instead of `x.paths`. The stub run passed 20/20, with both ADRs selected and the runbook excluded.
- **Negative control** on the same fixture: Vizzini `bc9104a` (before) selects `knowledge_docs: []`, while `ffe8e03` (after) selects both ADRs.

No mrman change is needed: grounding is review-time only and doesn't affect `pkg/risk`.
