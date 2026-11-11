---
title: Cost Attribution
description: Know what AI review costs per request, per run and per pull request, and who and what it was for, by joining Vizzini's attribution with LiteLLM's spend logs.
---

LiteLLM already records every request it proxies: model, tokens and dollar
cost. Vizzini adds the context that turns those rows into answers: which pull
request, which head commit, which run and mode, who asked, and through which
tool.

```sh
git diff main...HEAD | pr-review --mode review --format json \
  --pr "ado:dev.azure.com/org/project/repo#42" --iteration "$(git rev-parse HEAD)" \
  --requester "$BUILD_REQUESTEDFOR" --agent "azure-pipelines"
```

Each flag has an environment variable: `VIZZINI_RUN_ID`, `VIZZINI_PR`,
`VIZZINI_ITERATION`, `VIZZINI_REQUESTER` and `VIZZINI_AGENT`. From Go, put
`reviewer.Attribution` on the context with `reviewer.WithAttribution`. Every
mode (review, describe, improve, plan, command) then tags its requests.

### In CI

The CI system already knows the pull request and who started the run.

Azure Pipelines (a PR build of an Azure Repos repository):

```yaml
- bash: |
    org="$(basename "${SYSTEM_COLLECTIONURI%/}")"
    git diff "origin/${SYSTEM_PULLREQUEST_TARGETBRANCH#refs/heads/}...HEAD" | pr-review --mode review --format json \
      --pr "ado:dev.azure.com/$org/$SYSTEM_TEAMPROJECT/$BUILD_REPOSITORY_NAME#$SYSTEM_PULLREQUEST_PULLREQUESTID" \
      --iteration "$BUILD_SOURCEVERSION" --requester "$BUILD_REQUESTEDFOREMAIL" --agent azure-pipelines
```

In a PR build, `BUILD_SOURCEVERSION` is the pull request's merge commit, not
the source branch's head. It still changes with every push, which is what
`iteration` is for.

GitHub Actions:

```yaml
- run: |
    git diff "origin/${{ github.base_ref }}...HEAD" | pr-review --mode review --format json \
      --pr "github:github.com/${{ github.repository }}#${{ github.event.pull_request.number }}" \
      --iteration "${{ github.event.pull_request.head.sha }}" --requester "${{ github.actor }}" --agent "github-actions"
```

Keep `pr_key` stable across pushes (forge, host, repository and number), so
`vizzini_pr_costs` adds up a pull request's whole lifecycle.

## What each request carries

| Where | Content |
|---|---|
| `x-litellm-spend-logs-metadata` header | `vizzini.run_id`, `vizzini.pr_key`, `vizzini.iteration`, `vizzini.requester`, `vizzini.agent`, `vizzini.mode`, `vizzini.chunk` (`i/n`), `vizzini.attempt` and `vizzini.repair`. LiteLLM stores this in `metadata->'spend_logs_metadata'` |
| `x-litellm-tags` header | `vizzini` and `vizzini:<mode>`, for LiteLLM's tag-based spend reports |
| request body `user` | the requester, stored as LiteLLM's `end_user` |
| `traceparent` header | W3C trace context. The trace ID is the run ID (when it's a UUID), and each request gets its own span |
| request body `metadata` | the same keys, for OpenAI-compatible proxies other than LiteLLM |

`--run-id` defaults to a generated UUIDv7, which sorts by time. Each attempt
and repair is tagged, so the tokens spent on invalid answers are visible.

## What the reports carry

Every JSON report includes `run_id`, the attribution fields, and a `usage`
block. That covers review, describe, improve, plan and command reports:

```json
"usage": {"requests": 1, "retries": 0, "repairs": 0,
          "input_tokens": 4850, "output_tokens": 4055, "cached_input_tokens": 0,
          "cost_usd": 0.05025}
```

`cost_usd` is the sum of LiteLLM's `x-litellm-response-cost` header, so it
matches the spend logs exactly.

From Go, `ReviewOutput` carries these itself. For the other modes, put a
`reviewer.RunRecorder` on the context with `reviewer.WithRunRecorder`. Every
engine call then records a `RunInfo` (attribution, mode and usage), and
`reviewer.MarshalReportWithRun(result, &run)` adds it to the report.

## Setting up LiteLLM

- **Run LiteLLM with Postgres** (`DATABASE_URL`) so spend logs persist. Set
  `general_settings.store_prompts_in_spend_logs: false` so diffs are never
  stored.
- **Give models without a built-in price one.** Otherwise spend is recorded
  as 0:

  ```yaml
  - model_name: claude-sonnet-5-5
    litellm_params:
      model: anthropic/claude-sonnet-5-5
      input_cost_per_token: 0.000002     # check your provider's current prices
      output_cost_per_token: 0.00001
      cache_read_input_token_cost: 0.0000002
  ```

- **Create the views** from `deploy/litellm/views.sql` in this repository:
  `psql "$DATABASE_URL" -f deploy/litellm/views.sql`.

| View | One row per | Highlights |
|---|---|---|
| `vizzini_requests` | model request | run, PR, iteration, mode, chunk, attempt, repair, requester, cost and tokens |
| `vizzini_runs` | run | cost, tokens, money wasted on repairs and retries, and, with `vizzini_run_facts`, findings and cost per finding |
| `vizzini_pr_costs` | pull request | iterations, runs, requests, requesters and total cost across its lifecycle |

`vizzini_run_facts` holds what only the run knows: findings, blocking
findings, the gate's exit code and the risk score. Fill it from the review
reports in your CI step, or with a small collector.

## Trust and privacy

- **Identity is self-reported.** Requester and agent are whatever the caller
  says. For chargeback you can enforce, give teams or services their own
  LiteLLM virtual keys, and treat these fields as analytics.
- **The requester is personal data.** Set a retention period for spend logs
  (`maximum_spend_logs_retention_period`).
