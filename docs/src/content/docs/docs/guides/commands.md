---
title: Slash Commands
description: /explain, /refactor and /docstrings, scoped to a selection inside a review thread.
---

Inside a review thread, ask Vizzini about specific lines. Each command sends
the model **only** the hunks you selected and the thread so far, nothing else
from the diff.

| Command | Target | Answer |
|---|---|---|
| `/explain FILE:START-END [question]` | a line or range | A Markdown explanation of what the lines do and why they changed |
| `/refactor FILE:START-END [goal]` | a line or range | Replacement code that stays **inside** the selection |
| `/docstrings FILE [note]` | a file | Documentation comments for changed declarations, comments only |

Line ranges use new-file numbers. `a.go:44`, `a.go:44-45` and GitHub-style
`a.go:L44-L45` all work.

## From mrman (Go)

```go
cmd, err := reviewer.ParseCommand("/explain internal/store/users.go:44-45 is this injectable?")
if err != nil {
	return err // errors.Is(err, reviewer.ErrBadCommand)
}
res, err := engine.RunCommand(ctx, patch, cmd, []reviewer.ThreadMessage{
	{Author: "alice", Body: "Is this safe?"},
})
thread.Append(reviewer.FormatCommandResult(res))
```

## From the CLI

```sh
git diff main...HEAD | pr-review --mode command \
  --command "/refactor internal/store/users.go:45-46 bind the parameter" \
  --thread thread.json          # optional: [{"author": "...", "body": "..."}]
```

## Guarantees

- **Scoped.** If the file isn't in the diff, or the range has no changed
  lines, the command fails before any request is made.
- **Validated.** `/explain` answers must match `#Explanation`, and
  `/refactor` and `/docstrings` must match `#ImproveOutput`.
- **Anchored.** Suggestions must sit in the scoped hunks, and a `/refactor`
  suggestion that reaches outside the selection is sent back for repair.
- **Bounded.** Only the newest thread messages that fit 1500 tokens are
  sent.
