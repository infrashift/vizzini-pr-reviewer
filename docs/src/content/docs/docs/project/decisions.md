---
title: Design Decisions
description: The architecture decisions behind Vizzini, summarized from its ADRs.
---

Each decision is recorded in full as an ADR in the project's OKF wiki
(`.okfw/decisions/`). This page summarizes them.

## ADR-0001: An OKF wiki for project knowledge

Architecture notes, specs, ADRs, issues and daily notes live in an
[okfw](https://github.com/infrashift/okf-wiki)-managed wiki that is committed
with the code. Vizzini will also *consume* OKF (milestone 4.5), so using the
format on this project produces a real-world fixture.

## ADR-0002: Pure Go with `CGO_ENABLED=0`

Vizzini runs inside mrman and in minimal UBI9 images and QEMU microVMs, with
no libc coupling. That rules out cgo libraries such as tree-sitter bindings
and libgit2. The one exception is `make test`: Go's race detector needs cgo on
Linux, so cgo is enabled for that target alone. Lint and build stay
cgo-free.

## ADR-0003: CUE as the strict output boundary

LLM output is untrusted. Every answer is unified with closed CUE definitions
and decoded only if valid. CUE was chosen over JSON Schema because
cross-field constraints (`end_line: >=start_line`) are first-class, and the
same package will hold policy and risk configuration. CUE's runtime is
mutated during unification, so validation is serialized.

## ADR-0004: OpenAI-compatible transport through LiteLLM

One client (`go-openai`) talks to an internal LiteLLM proxy, which handles
model routing, keys and quotas. Switching models is a proxy configuration
change. Because JSON-mode support varies between backends, the real
guarantee is the CUE boundary.

## ADR-0005: Stateless, library-first API

`pkg/reviewer` takes a context and a patch reader and returns typed results.
It keeps no state, no globals and no hidden config, so mrman can embed it
directly. The CLI is a thin wrapper. New interfaces or dependencies need a
new ADR.

## ADR-0006: Offline token counting

Tokenizer encodings are pinned by checksum and pre-seeded into
`TIKTOKEN_CACHE_DIR`, instead of adding an embedded-loader dependency. See
[Running Offline](../../guides/offline/).

## ADR-0007: Findings anchored to the diff

After schema validation, every finding must name a file in its chunk and
overlap a changed hunk's new lines. Misplaced findings go back to the model
through the same repair loop. Chunk results merge deterministically: findings
by severity, `risk_score` as the maximum.

## ADR-0012: One finding per problem

A problem a model reports twice, once as a defect and once as the ADR it
violates, is folded into one Architecture finding with the higher severity.
Vizzini folds only pairs in the same file, with overlapping lines and a
shared citation. The report's `merged_findings` counts them. See
[Knowledge-Grounded Reviews](../../guides/knowledge/#one-finding-per-problem).


## ADR-0014: Repository context

With `--repo`, each chunk also gets the definitions of what it calls and the
callers of what it changes. They're found by language-family patterns in one
deterministic scan, within a token budget. Findings still have to point at
the diff. See [Repository Context](../../guides/repository-context/).

## ADR-0015: Cost attribution through LiteLLM

Vizzini doesn't keep a ledger. It attaches each request's who, what and why
(run, PR, head commit, mode, requester, agent, attempt, repair) to LiteLLM's
spend logs. It also adds a `usage` block to the review report and ships SQL
views that roll costs up per run and per PR. See
[Cost Attribution](../../guides/cost-attribution/).
## ADR-0010: Reports with an envelope; policy in garmr

JSON output carries `apiVersion: vizzini.infrashift.io/v1` and a `kind`, and
is validated against `#ReviewReport` or `#Report` before printing. Org policy
runs in [garmr](../../guides/garmr/) over that contract, and the built-in gate
stays the offline default.

## ADR-0009: Architecture findings cite provided documents

A fifth category, `Architecture`, must carry `citations` (a CUE conditional),
and every citation must name a knowledge-base document actually given to that
chunk. See [Knowledge-Grounded Reviews](../../guides/knowledge/).

## ADR-0008: Deterministic risk, conservative auto-approve

Risk is `max(diff-derived score, model score)`, so the model can raise it but
never lower it. Auto-approval also requires that no sensitive path (auth,
security, crypto, schema, migrations) was touched, nothing was skipped, and
something was actually reviewed. See [Risk & Auto-Approve](../../guides/risk/).

## ADR-0013: Complexity beyond Go

The risk score's decision points are now estimated per line for C-family
languages (C#, Java, TypeScript/JavaScript, C/C++, Kotlin, Swift, Rust, PHP,
…), Python and Ruby. A small lexer drops strings and comments, then counts
whole-word keywords and `&&`/`||`. Go keeps its exact count, and other files
count 0. See [Risk & Auto-Approve](../../guides/risk/).
