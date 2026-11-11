# CLAUDE.md — Go AI Code Review Engine

## Purpose (WHY)
Vizzini - A stateless, pure-Go AI code review library (`pkg/reviewer`) and CLI (`cmd/pr-review`) built for direct embedding into the `mrman` TUI (`bubbletea`, `bubbles`, `lipgloss`) and isolated execution inside Red Hat UBI9 containers or QEMU microVMs.

## Tech Stack & Pinned Libraries (WHAT)
- **Language:** Go 1.23+ with `CGO_ENABLED=0` (strictly pure Go, no cgo).
- **Schema & Policy Validation:** `cuelang.org/go` (CUE Go API).
- **Diff & Git Parsing:** `github.com/bluekeyes/go-gitdiff` and `github.com/go-git/go-git/v5`.
- **Token Counting:** `github.com/pkoukk/tiktoken-go`.
- **LLM Transport:** `github.com/sashabaranov/go-openai` configured to target an internal LiteLLM proxy or mock Anthropic endpoints.

## Verification & Workflow Commands (HOW)
Run these commands via GNU Make after every code change:
- `make test`     -> Runs `go test -v -race ./...`
- `make vet-cue`  -> Runs `cue vet ./schema/...`
- `make lint`     -> Runs `golangci-lint run`
- `make build`    -> Runs `CGO_ENABLED=0 go build -o bin/pr-review ./cmd/pr-review`

## Mandatory Execution Rules
1. **Work One Phase at a Time:** Read `PLAN.md`, implement only the current unchecked phase, run `make test && make vet-cue && make lint`, mark the phase checkboxes complete in `PLAN.md`, and stop for a commit.
2. **Test Integrity:** Never weaken or delete existing unit tests to make a failing implementation pass.
3. **Strict Schema Boundary:** Every LLM response must be unified and validated against `schema/review.cue` via `cuelang.org/go/cue/cuecontext` before returning data to callers.
4. **Minimal Abstractions:** Do not introduce unrequested interfaces, factory patterns, or third-party dependencies outside the pinned list.
5. **Keep the Wiki in Sync:** Each `PLAN.md` phase/milestone has a mirror issue in `.okfw/issues/` (linked from its `Tracking:` line). When a phase completes, tick that issue's acceptance criteria, add a **Resolution** section, swap the tag `open` → `closed`, record new decisions as ADRs in `.okfw/decisions/`, and run `okfw validate --all && okfw validate --okf`. Conventions: `.okfw/wiki-conventions--01a1c9b61519.md`.

<!-- BEGIN okfw -->
## Project knowledge base (okfw)

vizzini-pr-reviewer keeps a local-first OKF wiki in `.okfw/`: markdown concepts with
YAML frontmatter, linked to each other. The files are the whole database —
search and the link graph are computed from them on demand, so there is no
index to build and nothing to sync. Each directory's `index.md` is generated
and gives a cheap overview before you open anything (OKF section 8).

- **Search before you derive.** `okfw search "<query>"` / `okfw_search`. The
  wiki holds what the code does not say — why things are built the way they
  are, what was considered and rejected, dependency and security findings,
  operational context, open work. Check it before explaining a design,
  assessing risk, or running an analysis someone may already have recorded and
  adjudicated.
- **Browse top-down.** Each directory's `index.md` lists its concepts with
  their descriptions (`okfw_read {"ref":"<dir>/"}` over MCP). Open only what fits.
- **Search before you write**, too, so a new concept links to what already
  exists instead of duplicating it.
- **Link actively** — `[Title](/dir/file.md)` from the wiki root, or
  `./file.md`. Links are what makes the wiki more than a folder.
- **One idea per concept**, with a one-sentence `description`.
- **Use `okfw move` / `okfw delete`, never `mv` / `rm`.** `move` repairs every
  link to the concept and its own relative links, and both keep the `index.md`
  listings current; the shell commands break the links and leave the listings
  stale.
- **`status` is the OKF lifecycle field** (`draft`/`stable`/`deprecated`), not
  task state. Mark superseded knowledge rather than deleting it.
- **Custom metadata goes under `x:`** — the schema is closed and rejects any
  other unrecognized key.
- Don't edit `index.md` or `log.md`; okfw generates them.

Results are capped and say so: `truncated`, or `… N more`, means you did not
see everything — raise the limit or page with `--offset`/`"offset"`.

`okfw mcp` serves all of this as MCP stdio tools named `okfw_*`.
<!-- END okfw -->
