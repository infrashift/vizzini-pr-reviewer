---
title: Lint-Aware Reviews
description: Feed golangci-lint diagnostics into a review so the model explains real issues instead of guessing.
---

Linters are precise and models are articulate. Give Vizzini a golangci-lint
report and it adds the diagnostics on lines **your diff adds** to the review
prompt. The model then explains each one in plain terms and proposes a fix,
without inventing rules of its own.

## Run it

Run golangci-lint **from the repository root**, so its paths match the diff:

```sh
golangci-lint run --output.json.path=stdout --output.text.path=/dev/null ./... > lint.json
git diff origin/main...HEAD | pr-review --mode review --lint-json lint.json
```

golangci-lint v1's `--out-format=json` works as well, because both versions
write the same `Issues[]` shape.

## What reaches the model

Only diagnostics on a `+` line of the diff:

| Diagnostic on… | Sent? |
|---|---|
| A line the diff adds | Yes |
| A context line (unchanged) | No |
| A line outside every hunk | No |
| A file the diff doesn't touch | No |

When a large patch is split into chunks, each chunk gets only the diagnostics
on its own hunks. In the prompt they appear as a block before the diff:

```text
<lint>
internal/api/handler.go:4:13 errcheck: Error return value of `r.ParseForm` is not checked
internal/store/users.go:44:7 gosec: G202: SQL string concatenation
</lint>
```

The JSON output reports how many were included in `"diagnostics"`.

## From Go

```go
lint, _ := os.Open("lint.json")
defer lint.Close()
out, err := engine.Review(ctx, patch, reviewer.ReviewOptions{LintReport: lint})
```

A malformed report fails the review before any request is made.
