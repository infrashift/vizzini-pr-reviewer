---
title: CLI Reference
description: Flags, environment variables, output formats and exit codes for pr-review.
---

`pr-review` reads a unified diff, runs one Vizzini mode against your LiteLLM
proxy, and prints the validated result.

```sh
git diff main...HEAD | pr-review --mode review
pr-review --patch-file change.patch --mode describe --format json
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--patch-file` | `-` | Unified diff to read. `-` reads stdin |
| `--mode` | `review` | `describe`, `review` or `improve` |
| `--litellm-url` | `$VIZZINI_LITELLM_URL` | OpenAI-compatible API root, e.g. `http://litellm:4000/v1` |
| `--model` | `$VIZZINI_MODEL` | Model name as configured in LiteLLM |
| `--format` | `markdown` | `markdown` (for humans and mrman) or `json` |
| `--max-chunk-tokens` | `6000` | Token budget per LLM request |
| `--concurrency` | `4` | Parallel LLM requests |
| `--timeout` | `10m` | Overall deadline |
| `--skip-oversized` | `false` | Review mode: skip hunks larger than the token budget |
| `--fail-on` | `Critical,Major` | Review mode: comma-separated severities that make `pr-review` exit `2`, or `none` |

## Environment

| Variable | Used for |
|---|---|
| `VIZZINI_LITELLM_URL`, `VIZZINI_MODEL` | Defaults for the flags above |
| `VIZZINI_API_KEY` | Bearer token for the proxy (falls back to `LITELLM_API_KEY`). There is deliberately no flag, so the key never appears in `ps` |
| `TIKTOKEN_CACHE_DIR` | Pre-seeded tokenizer cache. See [Running Offline](../../guides/offline/) |

## Modes

| Mode | JSON output |
|---|---|
| `review` | `{summary, risk_score, findings[], excluded[], skipped[], chunks}` with findings sorted most severe first |
| `describe` | `{title, type, summary, changes[]}` |
| `improve` | `{suggestions: [{file, start_line, end_line, category, summary, existing_code, improved_code}]}` |

Every model answer is validated against [the schema](../review-schema/) and
anchored to the diff before it is printed.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Any error: bad flags, unreadable patch, proxy failure, or an answer the model could not repair |
| `2` | Review mode: at least one finding has a severity listed in `--fail-on` |

## Policy gate

The gate's default comes from `#PolicyGate` in `schema/review.cue`, which
blocks on `Critical` and `Major`. The review is always printed in full, and
the gate only decides the exit code, so a CI job can post the review and then
fail:

```sh
git diff origin/main...HEAD | pr-review --mode review > review.md
status=$?   # 0 pass, 2 blocked by policy, 1 error
```

```sh
pr-review --fail-on Critical            # block only on Critical
pr-review --fail-on Critical,Major,Minor
pr-review --fail-on none                # report, never block
```

An unknown severity such as `--fail-on High` is rejected before any request
is made.

Interrupting with Ctrl-C or SIGTERM cancels in-flight requests.
