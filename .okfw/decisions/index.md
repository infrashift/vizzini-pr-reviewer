# decisions

<!-- okfw:generated — maintained by okfw; edits here are overwritten -->

## Concepts

* [ADR-0001: Adopt an OKF wiki (okfw) for project knowledge](adr-0001-adopt-an-okf-wiki-okfw-for-project-knowledge--01a1c9b61524.md) - Project knowledge (architecture, specs, ADRs, issues, daily notes) lives in an okfw-managed OKF wiki at .okfw/.
* [ADR-0002: Pure Go with CGO_ENABLED=0](adr-0002-pure-go-with-cgo-enabled0--01a1c9b6152a.md) - Vizzini is strictly pure Go and ships as a static CGO_ENABLED=0 binary for UBI9 containers and QEMU microVMs.
* [ADR-0003: CUE as the strict LLM output boundary](adr-0003-cue-as-the-strict-llm-output-boundary--01a1c9b61530.md) - Every LLM response is unified with schema/review.cue via the CUE Go API before any data reaches callers.
* [ADR-0004: OpenAI-compatible transport through a LiteLLM proxy](adr-0004-openai-compatible-transport-through-a-litellm-proxy--01a1c9b61537.md) - LLM calls go through go-openai with a custom BaseURL targeting an internal LiteLLM proxy (or mock endpoints in tests).
* [ADR-0005: Stateless library-first API with minimal abstractions](adr-0005-stateless-library-first-api-with-minimal-abstractions--01a1c9b6153c.md) - pkg/reviewer is a stateless Go API embedded by mrman; the CLI is a thin wrapper; no unrequested interfaces or dependencies.
* [ADR-0006: Offline token counting via a pre-seeded TIKTOKEN_CACHE_DIR](adr-0006-offline-token-counting-via-a-pre-seeded-tiktoken-cache-dir--01a1c9be0019.md) - tiktoken encodings are checksum-pinned and pre-seeded into TIKTOKEN_CACHE_DIR so token counting never needs the network.
* [ADR-0007: Anchor findings to the diff and merge chunk results deterministically](adr-0007-anchor-findings-to-the-diff-and-merge-chunk-results-deterministically--01a1c9d9c501.md) - Findings and suggestions must point at files and new-line ranges the model saw; chunk results merge into one sorted, deterministic output.
* [ADR-0008: Deterministic risk score, model can only raise it, sensitive paths never auto-approve](adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md) - Risk is max(diff-derived score, model score); auto-approval also requires no sensitive-path files, nothing skipped and something reviewed.
* [ADR-0009: Architecture findings must cite provided knowledge documents](adr-0009-architecture-findings-must-cite-provided-knowledge-documents--01a1c9f25bc8.md) - A fifth category, Architecture, requires citations by CUE rule, and every citation must name an OKF document given to that chunk.

