---
title: Annotated Diff Format
description: How Vizzini renders diffs for the model, with explicit old and new line numbers.
---

Models are bad at counting lines from a `@@ -41,6 +41,7 @@` header, so
Vizzini doesn't ask them to. Every line of every hunk carries both line
numbers in a fixed-width gutter:

```text
### internal/store/users.go (modified)
@@ -41,6 +41,7 @@ func (s *Store) FindUser(ctx context.Context, name string) (*User, error) {
41 41       if name == "" {
42 42           return nil, ErrEmptyName
43 43       }
44    -     row := s.db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE name = $1", name)
   44 +     q := "SELECT id, name FROM users WHERE name = '" + name + "'"
   45 +     row := s.db.QueryRowContext(ctx, q)
45 46       var u User
46 47       return &u, row.Scan(&u.ID, &u.Name)
```

## Line layout

```text
<old> <new> <op> <text>
```

| Column | Content |
|---|---|
| `old` | Line number in the old file, blank for an added line |
| `new` | Line number in the new file, blank for a deleted line |
| `op` | `+` added, `-` deleted, blank for context |

Findings use the **new** column. For a problem on a deleted line, the model
is told to point at the nearest surviving new line.

## File headers

Each file starts with one line describing the change:

| Header | When |
|---|---|
| `### path (modified)` | Content changed |
| `### path (added)` / `(deleted)` | New or removed file |
| `### path (renamed from old/path)` | Renamed, possibly with edits |
| `### path (copied from old/path)` | Copied |
| `…, mode 100644 -> 100755)` | Appended when the file mode changed |
| `…, binary; content omitted)` | Binary file. The header is sent, the content is not |
| `### path (modified) [continued]` | A later part of a file split across chunks |
