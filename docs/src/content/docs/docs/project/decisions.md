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
