---
title: Embedding in Go
description: Use pkg/reviewer from a Go program such as the mrman TUI.
---

`pkg/reviewer` is the API the CLI wraps. An `Engine` holds no state between
calls and is safe for concurrent use, so a TUI can create one at startup and
share it.

```go
import "github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"

engine, err := reviewer.New(reviewer.Config{
	BaseURL: "http://litellm:4000/v1",
	APIKey:  os.Getenv("VIZZINI_API_KEY"),
	Model:   "review-model",
})
if err != nil {
	return err
}

out, err := engine.Review(ctx, patch, reviewer.ReviewOptions{})
if err != nil {
	return err
}
viewport.SetContent(reviewer.FormatReview(out)) // or render with glamour
```

## Methods

| Method | Returns |
|---|---|
| `Review(ctx, patch, opts)` | `*ReviewOutput`: summary, max risk score, sorted findings, excluded and skipped paths |
| `Describe(ctx, patch)` | `*PRDescription`, or `ErrNoChanges` if every file was excluded |
| `Improve(ctx, patch)` | `[]CodeSuggestion` sorted by file and line |
| `RunCommand(ctx, patch, cmd, thread)` | `*CommandResult` for a [slash command](../commands/) parsed with `ParseCommand` |
| `Plan(ctx, req)` | `*ImplementationPlan` for an issue; see [Pre-Code Planning](../planning/) |
| `Policy(failOn)` | A schema-validated `*PolicyGate`; `Blocking(findings)` returns the findings that fail it |

The package-level `OrderFilesByDependency(files)` orders a parsed diff for a
[dependency walkthrough](../walkthrough/).

`patch` is any `io.Reader` holding a unified diff (`git diff`, `git
format-patch`, or a forge's `.diff` URL body).

## Risk without a model

`pkg/risk` computes the deterministic [risk score](../risk/) on its own. It
links neither the LLM client nor the tokenizer (a test enforces this), so a
program can embed it to score every change in a list, cheaply:

```go
import "github.com/infrashift/vizzini-pr-reviewer/pkg/risk"

policy, err := risk.LoadPolicy(data) // nil → schema defaults; data from .vizzini/risk.json
if err != nil {
	return err
}
a, err := risk.AssessPatch(patch, policy, nil) // nil → default excludes, as in a review
// a.Score == base_risk_score, a.Band is low/medium/high, a.Files == risk_files
```

| Function | Use |
|---|---|
| `LoadPolicy(data)` / `DefaultPolicy()` | A schema-validated `*Policy`; `policy.Band(score)` and `policy.IsSensitive(weight)` |
| `AssessPatch(r, policy, exclude)` | Score a unified diff exactly as a review would |
| `Score(changes, policy)` | The same from `[]FileChange` (path, added and deleted line text, binary), for callers with their own diff model |
| `ScoreFile(&change, policy)` / `Combine(files, policy)` | Per-file results in your own order, then the total. Useful when one path appears twice, as in a patch series |
| `NewExcluder(exclude)` / `DefaultExcludes()` | Drop the same generated and vendored paths a review drops (nil → the defaults) before calling `Score` or `ScoreFile`, so your total matches `base_risk_score` |
| `ParseReport(data)` | The risk fields of a review someone else ran (`pr-review --format json`, or a marshaled `ReviewOutput`); `Divergence()` is how far the model's score sits above the base |

`auto_approve` from a parsed report is a suggestion. A human still decides
whether to approve.

## Configuration

Zero values select the defaults:

| Field | Default |
|---|---|
| `MaxChunkTokens` | 6000 |
| `Encoding` | `cl100k_base` |
| `Exclude` | `*.pb.go`, `*_gen.go`, `go.sum`, `vendor/**`, `*.lock` (an empty slice excludes nothing) |
| `Concurrency` | 4 parallel requests |
| `MaxRetries` / `MaxRepairs` | 3 / 2 (negative disables) |
| `Backoff` | 500ms, doubling |

## Rendering

`FormatReview`, `FormatDescription`, `FormatSuggestions` and `FormatRiskAssessment` produce Markdown
built for terminal viewports. It uses headings, lists and fenced code only:
no tables (they overflow narrow panes), no HTML, and no emoji (their cell
width varies between terminals and breaks lipgloss layouts). Code fences grow
longer than any backticks inside model-written code, so a suggestion can't
break out of its block.

## Errors

- Errors wrapping `schema.ErrInvalid` mean the model could not produce a
  valid, anchored answer within `MaxRepairs`.
- Proxy failures carry the underlying go-openai error.
- In a multi-chunk run, the error names the chunk (`chunk 2 of 3: …`) and
  cancels the others.
