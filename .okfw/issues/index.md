# issues

<!-- okfw:generated — maintained by okfw; edits here are overwritten -->

## Concepts

* [ISSUE-001: Diff parsing and token-aware chunking](issue-001-diff-parsing-and-token-aware-chunking--01a1c9b6154f.md) - Parse unified diffs, filter generated files, annotate line numbers, and pack hunks into token-bounded chunks.
* [ISSUE-002: CUE validation and LiteLLM pipeline](issue-002-cue-validation-and-litellm-pipeline--01a1c9b61556.md) - Author schema/review.cue, the embedded CUE validator, and the go-openai LiteLLM client with templates and retries.
* [ISSUE-003: Public API (pkg/reviewer) and CLI entrypoint](issue-003-public-api-pkg-reviewer-and-cli-entrypoint--01a1c9b6155c.md) - Expose Describe/Review/Improve via pkg/reviewer and a static pr-review CLI with json/markdown output.
* [ISSUE-004: Triage and severity policy gate](issue-004-triage-and-severity-policy-gate--01a1c9b61563.md) - CUE #PolicyGate with fail_on severities; pr-review exits 2 when blocking findings exist.
* [ISSUE-005: Hybrid verification: static analysis plus AI](issue-005-hybrid-verification-static-analysis-plus-ai--01a1c9b6156a.md) - Ingest golangci-lint JSON, keep diagnostics on modified lines, and inject them into the review prompt.
* [ISSUE-006: Semantic dependency walkthroughs](issue-006-semantic-dependency-walkthroughs--01a1c9b61571.md) - Order changed Go files by an AST-derived dependency DAG (Kahn's algorithm) for mrman's file tree.
* [ISSUE-007: Automated risk scoring and approvals](issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md) - Deterministic 0–100 blast-radius score from lines, complexity delta and CUE path weights; gated AutoApprove.
* [ISSUE-008: Knowledge base grounding (OKF RAG)](issue-008-knowledge-base-grounding-okf-rag--01a1c9b6157f.md) - Load OKF index.md and frontmatter, match diff paths to relevant knowledge, and require citations on architectural findings.
* [ISSUE-009: Interactive PR slash commands](issue-009-interactive-pr-slash-commands--01a1c9b61586.md) - Parse /explain, /refactor and /docstrings with file:line-range targets and route each to a scoped template.
* [ISSUE-010: Pre-code planning from issue payloads](issue-010-pre-code-planning-from-issue-payloads--01a1c9b6158d.md) - Turn an IssuePayload plus repo tree and OKF index into a CUE-validated #ImplementationPlan for coding agents.
* [ISSUE-011: Integrate the policy gate with garmr](issue-011-integrate-the-policy-gate-with-garmr--01a1c9e416ce.md) - Make pr-review JSON a first-class garmr input and ship a vizzini policy pack, keeping the built-in #PolicyGate as the offline default.
* [ISSUE-012: Multi-language lint input via SARIF](issue-012-multi-language-lint-input-via-sarif--01a1c9ed12c7.md) - Accept SARIF reports (ruff, Roslyn analyzers, Semgrep, CodeQL) alongside golangci-lint JSON so lint-aware reviews work beyond Go.
* [ISSUE-013: Explore what mrman gains from consuming Vizzini's risk score](issue-013-explore-what-mrman-gains-from-consuming-vizzinis-risk-score--01a1ca04b280.md) - Discovery: decide which mrman views would use risk_score, base/model split, risk_files and auto_approve, and what LLM-free API that needs.
* [ISSUE-014: Prove SARIF lint input with real ruff (Python) and Roslyn (C#) output](issue-014-prove-sarif-lint-input-with-real-ruff-python-and-roslyn-c-output--01a1ca11ff98.md) - Capture real ruff and Roslyn SARIF as fixtures and add Python and C# end-to-end proofs mirroring make e2e-lint.

