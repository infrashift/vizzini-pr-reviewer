---
id: "01a1caee-2572-7c47-9639-88673aef5cef"
title: "ISSUE-017: Typed review findings in pkg/risk for mrman"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "risk"
  - "mrman"
  - "api"
  - "closed"
created: "2026-11-11T08:06:07-06:00"
timestamp: "2026-11-11T08:06:07-06:00"
description: "risk.ParseReport also returns the review's findings, summary and knowledge docs as CUE-free types, so mrman can show Vizzini findings without linking pkg/reviewer."
---

# ISSUE-017: Typed review findings in pkg/risk for mrman

**Status:** closed · **Raised:** 2026-11-11 for the cross-tool UAT ([ISSUE-016](/issues/issue-016-cross-tool-uat-on-c-with-live-claude-sonnet-5-5-and-opus-5-5--01a1caee2585.md)) · **Builds on:** [ISSUE-015](/issues/issue-015-public-llm-free-risk-api-pkg-risk-for-mrman--01a1ca2a2474.md)

## Problem
mrman reads a `pr-review --format json` report through `risk.ParseReport`, but it only got the risk fields: `Findings` was a `json.RawMessage` that was checked for presence and then thrown away. To show findings, mrman would have had to import `pkg/reviewer`, which links the LLM client, or re-declare the finding shape and hope it never drifts.

## Acceptance criteria
- [x] `risk.Report` carries `Summary`, `Findings []risk.Finding` (never nil) and `KnowledgeDocs`
- [x] `risk.Finding` has the same fields, kinds and JSON names as `reviewer.Finding`. `TestRiskFindingMirrorsFinding` enforces this
- [x] A report from `MarshalReport` reads back with every finding intact (`TestMarshalReport_ReviewParsesAsRiskReport`)
- [x] `ParseReport` rejects findings an embedder cannot anchor or show: no file, `start_line < 1`, `end_line < start_line`, no severity, no title
- [x] `pkg/risk` stays a leaf: no LLM, tokenizer or `net/http` (ISSUE-015's dependency test)
- [x] The embedding guide documents the fields and says finding text is untrusted model output

## Resolution
`pkg/risk/report.go` decodes the findings into a string-typed `risk.Finding`. Severity and category stay strings, so the package needs neither the CUE schema nor `internal/schema`. A field added to one finding type and not the other fails the mirror test. Unknown fields are still ignored, so reports from newer Vizzini versions keep parsing.
