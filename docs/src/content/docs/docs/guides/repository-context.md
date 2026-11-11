---
title: Repository Context
description: Let the model see the definitions of what a change calls and the callers of what it changes, read from your checkout, so it can judge the change beyond the diff.
---

A diff shows what changed, but not what the changed code relies on. A model
that sees `orders.FindByCustomerAsync(prefix, ct)` can't check what that
method does, and a model that sees a method's signature change can't check
its callers. Give Vizzini the checkout, and it adds both to each chunk's
prompt:

```sh
git diff main...HEAD | pr-review --mode review --repo .
```

From Go, set `ReviewOptions.Repo` to an `fs.FS` of the repository at the
change's head, for example `os.DirFS(root)`.

## What is added

For each chunk, Vizzini reads the changed lines for symbols in two groups:

| Symbols | Taken from | Context added |
|---|---|---|
| **Used**: functions and methods called, types instantiated with `new` | added lines | the symbol's **definition**, from its declaration to the end of its body |
| **Defined**: functions, methods and types declared | added or deleted lines | up to three **call sites**, each with two lines either side |

The snippets go in a `<context>` block, each labelled with its path, lines,
kind and symbol, and with line numbers. The model is told to use them to
judge the change, and that every finding must still point at lines of the
diff. A finding on a context line goes back for repair, like any finding
outside the diff. The report's `context_snippets` counts what was given.

## What is left out

- **Common names:** keywords, names shorter than three characters, and very
  common calls (`len`, `String`, `Println`, `log`, `push`, …).
- **Symbols the chunk defines itself:** their definition is already in it.
- **Code already shown:** regions the chunk's diff already shows.
- **Directories and files skipped:** `.git`, `vendor`, `node_modules`,
  `dist`, `build`, `bin`, `obj`, virtualenvs and other dot-directories, files
  matching the review's exclude globs, files over 512 KiB, and binary files.
- **Other languages:** files of a different language family from the changed
  file.

## Budget and order

Each chunk gets at most `MaxContextTokens` of context (default 1,500). The
order is:
1. Definitions before callers.
2. Within each, the nearest files first: the same directory as a changed
   file, then the same top-level directory, then anywhere else.
3. Then by path and line.

Each symbol gets at most two definitions and three callers. The scan reads
the repository once per review, in a fixed order, capped at 20,000 source
files and 64 MiB.

## Languages

Retrieval uses declaration and call patterns per language family. It is not
a parser:

| Family | Extensions |
|---|---|
| Go | `.go` |
| C-family | `.c` `.h` `.cc` `.cpp` `.cxx` `.hpp` `.cs` `.java` `.kt` `.kts` `.swift` `.php` `.scala` `.dart` `.groovy` |
| JavaScript / TypeScript | `.js` `.jsx` `.mjs` `.cjs` `.ts` `.tsx` `.mts` `.cts` |
| Python | `.py` `.pyi` |
| Ruby | `.rb` |
| Rust | `.rs` |

The patterns can miss unusual declarations, and they match by name, so an
overloaded or common name can bring back an unrelated definition. The budget
and the per-symbol caps keep that cheap. The model still has to judge
relevance, like a reviewer skimming search results.
