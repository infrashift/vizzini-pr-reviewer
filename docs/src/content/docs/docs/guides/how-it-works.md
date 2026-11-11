---
title: How a Review Works
description: The path from a unified diff to schema-validated findings.
---

A review runs through five stages. Each stage is a small, separately tested
package, and nothing is stored between runs.

```text
unified diff
  → parse      internal/diff      drop generated files, label line numbers
  → chunk      internal/chunker   pack hunks into ≤ 6000-token chunks
  → prompt     internal/llm       render the review template per chunk
  → complete   internal/llm       JSON-mode call through LiteLLM, retries
  → validate   internal/schema    unify with schema/review.cue, decode
```

## 1. Parse

`internal/diff` parses the patch with
[go-gitdiff](https://github.com/bluekeyes/go-gitdiff). By default it drops
paths that only add noise:

| Pattern | Matches |
|---|---|
| `*.pb.go`, `*_gen.go` | Generated Go code, at any depth |
| `go.sum`, `*.lock` | Lockfiles, at any depth |
| `vendor/**` | Vendored code at the repository root |

A pattern without a `/` matches the file's base name anywhere in the tree. A
pattern with a `/` is anchored at the repository root, and `**` matches any
number of directories. The dropped paths are reported, not silently lost.

Every hunk line then gets its old and new line number. See
[Annotated Diff Format](../../reference/diff-format/).

## 2. Chunk

`internal/chunker` counts tokens with
[tiktoken-go](https://github.com/pkoukk/tiktoken-go) (`cl100k_base` by
default, `o200k_base` also available). It packs files in order, filling each
chunk greedily up to `MaxChunkTokens` (default 6000):

- A file that fits is **never** split.
- A file that doesn't fit is split **between hunks**, never inside one. Each
  later part repeats the file header with `[continued]`.
- A single hunk larger than the whole budget becomes its own chunk with
  `Oversized: true`, and the caller decides what to do with it.

The token counts are exact, not estimated. Every rendered piece starts with a
non-whitespace marker (`###` or `@@`), so the tokenizer splits the text the
same way whether a piece stands alone or sits inside a chunk.

## 3. Prompt

Prompts are `text/template` files embedded in the binary. The review
template lists the allowed severities and categories straight from the same
Go constants the validator uses. A test fails if those constants ever drift
from the CUE schema.

## 4. Complete

`internal/llm` calls an OpenAI-compatible endpoint (your LiteLLM proxy) in
JSON mode:

- **Transient failures** (network errors, 408, 429, 5xx) are retried with
  doubling backoff, and a cancelled context stops the wait immediately.
- **Client errors** (other 4xx) fail at once. Retrying a bad request or a bad
  key would only burn quota.
- A model that wraps its JSON in a Markdown code fence gets the fence
  stripped.

## 5. Validate and repair

The answer is unified with the closed `#ReviewOutput` definition in
[`schema/review.cue`](../../reference/review-schema/). If it fails,
the validator's error goes back to the model in the same conversation:

```text
Your previous answer was rejected by the schema validator:
schema validation failed: #ReviewOutput.findings.0.severity: conflicting values "Critical" and "High"; #ReviewOutput.findings.0.severity: conflicting values "Major" and "High"; …

Return the corrected JSON object only, with no prose and no code fences.
```

This happens up to `MaxRepairs` times (default 2). If the model still can't
produce a valid answer, the caller gets an error wrapping `schema.ErrInvalid`,
never unvalidated data.
