---
title: Contributing
description: The development loop, quality gate and knowledge-base conventions for Vizzini.
---

## The loop

1. **Pick the phase.** Work on one roadmap phase or milestone at a time. Its
   issue lives in `.okfw/issues/`, and `okfw search --path issues --tag open`
   lists open work.
2. **Implement only that scope.** Use the pinned libraries. Don't add
   unrequested interfaces, factories or dependencies. A new dependency needs
   an ADR first.
3. **Run the gate:** `make test && make vet-cue && make lint`. Never weaken or
   delete a test to get to green.
4. **Update the docs and the wiki.** Tick the phase in `PLAN.md` and close its
   issue with a Resolution section. Record decisions as ADRs. Run
   `make wiki`.
5. **Commit** the code and its wiki updates together, one commit per phase.

## Ground rules

- **Pure Go.** `CGO_ENABLED=0` for everything except the race detector in
  `make test`.
- **Schema first.** No LLM output reaches a caller without passing
  `schema/review.cue`. If you add a new kind of output, add its CUE
  definition first.
- **Tests stay offline.** The chunker tests block the network. Seed the
  tokenizer cache with `make tiktoken-cache`.

## The knowledge base

Design knowledge lives in `.okfw/`, an
[Open Knowledge Format](https://github.com/infrashift/okf-wiki) wiki managed
by `okfw`. Search it before re-deriving a decision:

```sh
okfw search "chunking"
okfw search --type ADR
```

Create concepts with `okfw create`, never by hand-writing frontmatter, and
move or delete them with `okfw move` and `okfw delete` so links stay intact.

## This site

```sh
make docs-dev     # http://localhost:4321/vizzini-pr-reviewer/
make docs-build   # what CI and the Pages deploy run
```

The landing page demo comes from Vizzini's own code. Regenerate it with
`go run ./docs/src/demo/gen.go` after changing the diff renderer or the
schema.
