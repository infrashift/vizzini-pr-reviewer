---
title: Pre-Code Planning
description: Turn an issue into a validated, phased implementation plan for a coding agent.
---

Before any code exists, Vizzini can turn an issue into a plan a coding agent
can execute step by step: which files change, in what order, and which
commands prove the work is done.

```sh
git ls-files > tree.txt
pr-review --mode plan --issue issue.json --tree tree.txt --okf-index .okfw/index.md > PLAN.md
```

```json
{ "title": "Retry transient LiteLLM failures", "body": "429s fail the whole review.", "labels": ["enhancement"] }
```

From Go:

```go
plan, err := engine.Plan(ctx, &reviewer.PlanRequest{
	Issue:    reviewer.IssuePayload{Title: title, Body: body, Labels: labels},
	FileTree: paths,           // e.g. from git ls-files
	OKFIndex: string(index),   // the knowledge base's root index.md
})
fmt.Print(reviewer.FormatPlan(title, plan))
```

## What comes back

`#ImplementationPlan` in `schema/review.cue`:

| Field | Contents |
|---|---|
| `summary` | The approach and why |
| `affected_files` | `{path, change: create \| modify \| delete, reason}`, at least one |
| `phases` | Named phases, each with at least one `{title, description, files}` step |
| `verification` | Shell commands that prove the work, at least one |
| `risks`, `references` | Optional. References are knowledge-base concept paths |

`FormatPlan` renders it in the same shape as Vizzini's own `PLAN.md`: phases
with `- [ ]` checkbox steps and a verification block, ready to hand to an
agent.

## Grounded, not guessed

The plan is checked against what you gave it, and violations go back to the
model:

- `modify` and `delete` paths must exist in the tree, and `create` paths must
  not.
- Every step's files must be listed in `affected_files`.
- `references` may only name concepts the OKF index links to.

The tree is capped at about 4000 tokens (the prompt notes how many paths were
omitted) and the index at 1500. Without a tree, the existence checks are
skipped.
