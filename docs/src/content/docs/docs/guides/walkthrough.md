---
title: Dependency Walkthroughs
description: Order a diff's files so reviewers read data models first, then logic, then UI.
---

Reading a pull request in the order git lists it (alphabetically) often means
reading the UI before the model it renders. `OrderFilesByDependency` reorders
a diff so each Go file comes **after** the changed files it depends on. mrman
uses it for its file tree.

```go
import (
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

files, _, err := gitdiff.Parse(patch)
if err != nil {
	return err
}
for _, f := range reviewer.OrderFilesByDependency(files) {
	fmt.Println(f.NewName)
}
// model/user.go
// service/users.go
// ui/profile.go
```

## What counts as a dependency

| File A depends on file B when… | Example |
|---|---|
| A imports B's package | `ui/profile.go` imports `example.com/app/service` |
| A and B share a directory and A uses a type B declares | `handler.go` has a `*Service` field declared in `service.go` |

Only the diff is read, so no checkout is needed. New files are parsed in full
with `go/parser`. For modified files, the new side of each hunk is parsed if
it forms a valid file. Otherwise it is scanned token by token, which still
finds imports, `type` declarations and references in a fragment.

## Ordering rules

- Each file is placed only after the files it depends on (Kahn's topological
  sort).
- Ties go to the shallower path, then alphabetical order, so the result is
  identical for every input order.
- Go allows type cycles within a package. A cycle is broken the same way:
  the shallowest path comes first.
- Non-Go files (docs, CUE, SQL) follow the Go files in their original diff
  order.
