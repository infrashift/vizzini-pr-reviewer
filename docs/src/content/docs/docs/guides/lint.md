---
title: Lint-Aware Reviews
description: Feed golangci-lint or SARIF diagnostics into a review so the model explains real issues instead of guessing.
---

Linters are precise and models are articulate. Give Vizzini a golangci-lint
report and it adds the diagnostics on lines **your diff adds** to the review
prompt. The model then explains each one in plain terms and proposes a fix,
without inventing rules of its own.

## Run it

Run golangci-lint **from the repository root**, so its paths match the diff:

```sh
golangci-lint run --output.json.path=stdout --output.text.path=/dev/null --show-stats=false ./... > lint.json
git diff origin/main...HEAD | pr-review --mode review --lint-json lint.json
```

golangci-lint v1's `--out-format=json` works as well, because both versions
write the same `Issues[]` shape. Without `--show-stats=false`, v2 appends a
plain-text summary after the JSON. Vizzini ignores it, but other JSON tools
may not.

## Other languages: SARIF

Any tool that writes [SARIF 2.1.0](https://sarifweb.azurewebsites.net/) works:
ruff for Python, Roslyn analyzers for C#, Semgrep, CodeQL and many more. Pass
the report the same way; the format is detected automatically, and
`--lint-json` can be repeated for repositories with several linters:

```sh
ruff check --output-format=sarif . > ruff.sarif
golangci-lint run --output.json.path=stdout --output.text.path=/dev/null --show-stats=false ./... > go.json
git diff main...HEAD | pr-review --mode review --lint-json go.json --lint-json ruff.sarif
```

### Verified tools

These were captured and proven end to end (`make e2e-lint-python`,
`make e2e-lint-csharp`):

| Language | Tool (version verified) | Produce SARIF with | Notes |
|---|---|---|---|
| Python | ruff 0.17.4 | `ruff check --output-format=sarif .` | Writes absolute `file://` URIs. Run `pr-review` from the same repo root. Security rules are the `S` group (`select = ["F", "S"]`) |
| C# | Roslyn, C# compiler 5.9.0 (.NET SDK 10.0.404) | `<ErrorLog>roslyn.sarif,version=2.1</ErrorLog>` in the project, then `dotnet build` | Writes absolute `file://` URIs. Security analyzers are off by default; enable them with `<AnalysisLevelSecurity>latest-all</AnalysisLevelSecurity>`. CA2100 fires on `DbCommand.CommandText`, not through `IDbCommand` |

If `dotnet build` runs in a container, mount the repository at the **same
absolute path** (`-v "$PWD:$PWD" -w "$PWD"`), so the URIs Roslyn writes match
where `pr-review` runs.

How a SARIF result is read:

| SARIF | Becomes |
|---|---|
| `tool.driver.name` | the linter name |
| `ruleId` + `message.text` | `S608: Possible SQL injection…` (falls back to the rule's description) |
| first `physicalLocation` | file, line and column |
| `file://` URIs, `uriBaseId` + `originalUriBaseIds` | repository-relative paths (`pr-review` resolves against its working directory, so run it from the repo root) |
| `kind: pass` / `notApplicable`, suppressions in effect, no line | skipped |

Absolute locations outside the repository, such as SDK files, are ignored.

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
goLint, _ := os.Open("go.json")
ruff, _ := os.Open("ruff.sarif")
out, err := engine.Review(ctx, patch, reviewer.ReviewOptions{
	LintReports: []io.Reader{goLint, ruff},
	LintRoot:    repoRoot, // absolute; resolves SARIF file:// URIs
})
```

A malformed report fails the review before any request is made.

## See it work

```sh
make e2e-lint          # Go: golangci-lint JSON
make e2e-lint-python   # Python: ruff SARIF (uses uvx ruff if ruff isn't installed)
make e2e-lint-csharp   # C#: Roslyn SARIF from dotnet build in the .NET SDK container
```

This builds a throwaway Go repo with a pre-existing lint issue, makes a change
that adds new ones, runs the real golangci-lint and `pr-review`, and checks
four things: only the new issues reach the prompt, every finding is anchored
to its line, the gate exits 2, and auto-approve stays off. Without
`VIZZINI_LITELLM_URL`, a stand-in model answers from the `<lint>` block alone.
Point it at your proxy to see a real model explain the issues.
