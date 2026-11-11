---
title: Review Schema
description: The CUE contract every LLM review answer must satisfy before Vizzini returns it.
---

`schema/review.cue` is the single source of truth for what a review looks
like. The binary embeds it, `make vet-cue` checks it, and every model answer
is unified with it before any caller sees data.

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
	findings: [...#Finding]
}
```

## Fields

| Field | Meaning |
|---|---|
| `file` | Path exactly as shown in the diff's `###` header |
| `start_line`, `end_line` | **New-file** line numbers. `end_line` may not precede `start_line` |
| `severity` | `Critical`: exploitable or loses data. `Major`: wrong behavior users will hit. `Minor`: an edge-case bug or meaningful debt. `Trivial`: a small cleanup |
| `category` | `Security`, `Performance`, `Logic` or `Maintainability` |
| `title` | A short imperative headline |
| `body` | What is wrong, why it matters, how it fails |
| `suggestion` | Optional replacement code or a concrete fix |
| `risk_score` | The model's 0–100 estimate. A deterministic score is planned |

## Rules worth knowing

- **Definitions are closed.** An extra field anywhere, such as `confidence`
  or `verdict`, is rejected rather than ignored.
- **Exact spellings.** `"High"` or `"major"` is not a severity. The answer
  goes back to the model for repair.
- **Integers are integers.** `5.5` or `"40"` fails `int`.
- **An omitted `findings` field means no findings.** CUE treats the open list
  `[...#Finding]` as concrete with default `[]`.

## Planned extensions

The same CUE package will carry the severity policy gate (`fail_on`), path
risk weights for blast-radius scoring, citations of OKF knowledge-base
documents, and a schema for implementation plans. See the
[roadmap](../../project/roadmap/).
