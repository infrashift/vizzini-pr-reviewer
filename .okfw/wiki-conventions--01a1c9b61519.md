---
id: "01a1c9b6-1519-7367-b1ff-b8df0d853563"
title: "Wiki conventions"
type: "Guide"
tags:
  - "conventions"
  - "meta"
created: "2026-11-11T02:25:15-06:00"
timestamp: "2026-11-11T02:25:15-06:00"
description: "Layout and authoring conventions for architecture notes, specs, ADRs, issues and daily notes in this wiki."
---

# Wiki conventions

This is how Vizzini's project knowledge is organized in its OKF wiki (`.okfw/`). Create concepts with `okfw create` or the okfw MCP `create` tool, and never hand-write frontmatter. Search before you write, link actively, and keep one idea per concept with a one-sentence `description`.

## Layout
| Path | Type | Contents |
|---|---|---|
| `architecture/` | `Architecture` | System overview, package boundaries, data flow, embedding targets. |
| `specs/` | `Spec` | Technical specifications such as schemas and API contracts. A spec describes intent; the code (e.g. `schema/review.cue`) is the implementation. |
| `decisions/` | `ADR` | Architecture decision records. |
| `issues/` | `Issue` (kind `issue`) | Tracked work: one issue per `PLAN.md` phase or milestone, plus bugs and design debt found along the way. |
| `plans/` | `Plan` | Reserved for multi-issue plans that go beyond `PLAN.md`. Create it the first time it's needed. |
| `daily/` | daily note | `okfw daily`. A journal of what changed, what was learned and what's next. |

## PLAN.md ↔ issues
`PLAN.md` stays the phase checklist that CLAUDE.md rule 1 depends on. Each phase/milestone has a mirror issue (ISSUE-001 … ISSUE-010), and each `PLAN.md` heading has a `Tracking:` line pointing at it. When a phase is done, tick **both**: the `PLAN.md` checkboxes and the issue's acceptance criteria.

## ADRs (`decisions/`)
- The title is `ADR-NNNN: <decision>`, numbered sequentially. Numbers are never reused.
- `okfw create "ADR-NNNN: <title>" --path decisions --type ADR --tags adr,<topic> -d "<one sentence>" --body "<markdown>"`
- Body: **Status** (Proposed / Accepted / Superseded by ADR-XXXX), **Date**, **Source**, then **Context**, **Decision**, **Consequences**, **Alternatives considered**.
- To supersede an ADR: write the new ADR with a link to the old one, update the old one's Status line, and run `okfw set-lifecycle <id> deprecated`.
- A new third-party dependency outside the pinned list in CLAUDE.md needs an ADR first.

## Issues (`issues/`)
- The title is `ISSUE-NNN: <summary>`, numbered sequentially.
- `okfw create "ISSUE-NNN: <title>" --kind issue --path issues --type Issue --tags issue,open,phase-<n>,<area> -d "<summary>" --body "<markdown>"`
- Body: a **Status/Phase** line, **Description**, **Acceptance Criteria** (checkboxes), **Notes**, **Related**.
- okfw has no issue-status field, so state is carried by tags: `open` → `in-progress` → `closed`, swapped with `okfw remove-tags` / `okfw add-tags`. Keep the Status line in sync. When closing, add a **Resolution** section.
- To find open work: `okfw search --path issues --tag open`.

## Housekeeping
- Use `okfw move` / `okfw delete`, never `mv` / `rm`. Those okfw commands repair links and keep the `index.md` listings current.
- `status` is the OKF lifecycle field (`draft` / `stable` / `deprecated`), not task state. Mark superseded knowledge `deprecated` instead of deleting it.
- Don't edit `index.md` or `log.md`; okfw generates them.
- Before committing wiki changes: `okfw validate --all && okfw validate --okf`.
