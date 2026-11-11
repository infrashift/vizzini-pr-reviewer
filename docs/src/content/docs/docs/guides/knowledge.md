---
title: Knowledge-Grounded Reviews
description: Give Vizzini your OKF design documents so reviews can cite the decisions a change violates.
---

Point Vizzini at an [Open Knowledge Format](https://github.com/infrashift/okf-wiki)
bundle, such as an okfw wiki's `.okfw/` directory. For each chunk of the diff
it loads only the documents that govern the changed code, and the model can
flag a change that breaks one of your design decisions, **citing it by ID**.

```sh
git diff main...HEAD | pr-review --mode review --knowledge .okfw
```

```go
out, err := engine.Review(ctx, patch, reviewer.ReviewOptions{Knowledge: os.DirFS(".okfw")})
```

## Telling Vizzini what a document governs

Add any of these to a concept's frontmatter. Under okfw, custom keys belong
under `x:`; top-level keys work too, for other OKF producers.

```yaml
---
id: "adr-0001-store"
title: "ADR-0001: Parameterized SQL only"
type: "ADR"
tags: ["adr", "persistence"]
x:
  paths: ["internal/store/**"]   # repository globs
  domain: "store"                # package or directory names
---
```

| Key | Matches when… | Priority |
|---|---|---|
| `paths` | a glob matches a changed file (`**` spans directories) | 1 |
| `domain` | it names a changed file's Go package or one of its directories | 2 |
| `tags` | same as domain, as a weaker signal | 3 |

A document with none of these is never selected, which keeps general notes
out of every prompt. Documents are found by following `index.md` listings from
the bundle root (OKF progressive disclosure), so concepts no index lists are
not loaded.

## Citations are checked

Findings gain a fifth category, **Architecture**, for a change that violates
a provided document. The schema requires such a finding to cite it:

```json
{
  "file": "internal/store/users.go", "start_line": 44, "end_line": 45,
  "severity": "Major", "category": "Architecture",
  "title": "SQL built by concatenation",
  "body": "ADR-0001 requires parameterized queries in the store.",
  "citations": ["adr-0001-store"]
}
```

Every citation must be the ID of a document that chunk was actually given. An
uncited Architecture finding, or a made-up ID, goes back to the model with the
list of allowed IDs, just like a finding on a line that isn't in the diff.

### One finding per problem

A problem can be both a defect and a design violation, for example an
`execSync` command built from input that an ADR also forbids. Some models
report it twice: once as Security and once as Architecture. The prompt asks
for a single Architecture finding that carries the citation. When a model
still sends both, Vizzini folds the pair into one finding. It folds only when
all of these hold:
- both findings are in the same file and their lines overlap;
- exactly one of them is Architecture;
- they cite at least one document in common.

The merged finding stays **Architecture**, so garmr's VZ-002 still reports
the violated decision. It also takes:
- the more severe severity;
- the specific problem's title, and both bodies;
- the span covering both findings;
- the union of the citations.

A different problem on the same line doesn't cite that document, so it stays
separate. The review's `merged_findings` field counts the folded pairs.

## Budget

Each chunk gets at most `MaxKnowledgeTokens` (default 2000) of documents,
strongest match first. If the best match alone exceeds the budget it is
truncated and marked `[…truncated]`, and weaker matches are dropped. The
review's `knowledge_docs` field lists what was used.
