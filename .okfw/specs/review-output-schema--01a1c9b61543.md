---
id: "01a1c9b6-1543-70dc-a659-f0e527e911e3"
title: "Review output schema"
type: "Spec"
tags:
  - "spec"
  - "cue"
  - "schema"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:49:18-06:00"
description: "The CUE contract (#Severity, #Category, #Finding, #ReviewOutput) every LLM review response must satisfy."
status: "stable"
---

# Review output schema

**Implements:** [ADR-0003: CUE as the strict LLM output boundary](/decisions/adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md) · **Delivered by:** [ISSUE-002: CUE validation and LiteLLM pipeline](/issues/issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md)

The canonical file is `schema/review.cue` (package `review`). Implemented in Phase 2. If this page and the file disagree, the file is correct.

```cue
package review

#Severity: "Critical" | "Major" | "Minor" | "Trivial"
#Category: "Security" | "Performance" | "Logic" | "Maintainability"

#Finding: {
    file:        string & !=""
    start_line:  int & >=1
    end_line:    int & >=start_line
    severity:    #Severity
    category:    #Category
    title:       string & !=""
    body:        string & !=""
    suggestion?: string
}

#ReviewOutput: {
    summary:    string
    risk_score: int & >=0 & <=100
    findings:   [...#Finding]
}
```

## Semantics
- `start_line` / `end_line` refer to **new-file** line numbers from the annotated hunk (Phase 1 parser). Findings on lines outside the diff should be rejected or remapped (open question for Phase 2/3).
- An omitted `findings` field decodes to an empty list: CUE treats `[...#Finding]` as concrete with default `[]`.
- Definitions are closed. Unknown fields at any level are rejected, and the error is sent back to the model for repair (`internal/llm`).
- Severity drives the policy gate: `fail_on` defaults to `["Critical", "Major"]` (Milestone 4.1).
- `risk_score` is the LLM's estimate. Milestone 4.4 adds a deterministic base score and `AutoApprove`.

## Planned extensions
- `#PolicyGate` (4.1), path risk weights (4.4), an OKF citation field on `#Finding` (4.5), `#ImplementationPlan` (4.7).
