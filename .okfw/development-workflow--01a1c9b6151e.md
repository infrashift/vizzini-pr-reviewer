---
id: "01a1c9b6-151e-7df2-84f0-f1d8344c96d9"
title: "Development workflow"
type: "Guide"
tags:
  - "conventions"
  - "workflow"
  - "meta"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "The one-phase-at-a-time implement, verify, update-wiki, commit loop for Vizzini."
---

# Development workflow

Every change follows this loop. Work isn't reported or committed while any step is red.

1. **Pick the phase.** Read `PLAN.md`, take the first unchecked phase, and open its mirror issue (`okfw search --path issues --tag open`). Tag it in progress: `okfw remove-tags <id> open && okfw add-tags <id> in-progress`.
2. **Implement only that phase.** Use the pinned libraries in CLAUDE.md. Don't add unrequested interfaces, factories or dependencies; a new dependency needs an ADR first.
3. **Verify.** Run `make test && make vet-cue && make lint`, and `make build` once the CLI exists. Never weaken or delete existing tests to get to green.
4. **Update the docs.** Tick the phase checkboxes in `PLAN.md`. In the issue, tick its acceptance criteria, add a **Resolution** section, and swap the tag `in-progress` → `closed`. Record new decisions as ADRs and file new findings as issues. Then run `okfw validate --all && okfw validate --okf`.
5. **Journal.** Add a line to today's `okfw daily` note.
6. **Stop for a commit.** Code and its wiki updates go together, one commit per phase/milestone.

See [Wiki conventions](/wiki-conventions--01a1c9b61519.md).
