# Why Vizzini

Vizzini is an AI code reviewer built for teams that need review automation they can **govern**, **audit** and **run on their own infrastructure**. It reviews pull requests with any model behind a LiteLLM proxy. It posts only what a committed policy allows. It remembers what it said without a database. It costs only what you can attribute.

The model is the replaceable part. Everything around the model decides whether AI review is safe to automate:
- validation;
- authorization;
- memory;
- the rules a team agrees on;
- cost.

That is where Vizzini puts its effort.

## At a glance

| Capability | What it does | Business value |
|---|---|---|
| [Schema-validated reviews](#1-reviews-you-can-trust-mechanically) | Every model answer is checked against a CUE schema and anchored to changed lines, or repaired | No malformed or misplaced comments reach developers. Downstream automation can rely on the output. |
| [Grounded reviews](#2-reviews-grounded-in-your-code-your-linters-and-your-decisions) | Linter diagnostics, callers and definitions from the checkout, and your design decisions (ADRs) go to the model | Findings explain real issues and cite your own architecture rules, instead of generic advice |
| [Policy-authorized posting](#3-a-bot-that-cannot-authorize-itself) | The bot posts, resolves or replies only when a committed garmr policy on the base branch allows it, and every decision is audited | AI write access becomes a reviewed, versioned and auditable grant. Compliance teams can sign off on it. |
| [Memory without a database](#4-memory-without-a-database) | The bot reads its own earlier threads back from the PR, reviews only what changed, never repeats itself, and resolves what was fixed | Less noise, fewer tokens, and nothing to operate or back up |
| [Reviewed suppression rules](#5-noise-control-that-goes-through-review) | Dismissals become suggested rules that take effect only after a merge to the base branch | False positives go away for good, and a PR cannot silence its own findings |
| [Disputes with a person as the arbiter](#6-disagreement-without-a-deadlock) | `/vizzini reconsider` lets the bot withdraw its own finding. Anything else stays open for a person. | Developer and agent workflows can argue with the reviewer without giving the reviewer, or the agent, the last word |
| [Commands in the thread](#7-a-reviewer-you-can-talk-to) | `/vizzini explain`, `refactor`, `docstrings` and `reconsider`, answered on the next run or immediately with the optional webhook receiver | Reviewers get answers in context. Developers get scoped refactors and docs. |
| [Risk scoring and a policy gate](#8-deterministic-risk-and-merge-policy) | A deterministic blast-radius score, which the model can raise but never lower, and garmr merge decisions from the JSON report | Low-risk changes can flow, high-risk ones get human attention, and the rules are code |
| [Cost attribution](#9-cost-you-can-explain) | Every model call is attributed to its run, mode, PR and requester in LiteLLM's spend logs, without storing prompts | AI review spend is explainable per PR and per person, with budgets enforced at the proxy |
| [Runs anywhere, model-agnostic](#10-your-models-your-infrastructure-your-forge) | Pure-Go library and CLIs with no cgo. Any model through LiteLLM. GitHub, GitLab (including self-managed) and Azure DevOps. Offline-capable. | No vendor lock-in, data stays on your infrastructure, and it fits regulated and air-gapped environments |
| [Beyond review](#11-beyond-review-describe-improve-plan-walk-through) | PR descriptions, improvement suggestions, phased implementation plans from issues, and dependency-ordered walkthroughs | One engine for the whole PR lifecycle, including the start of agentic coding |
| [Verified end to end](#12-verified-in-a-public-lab-not-just-unit-tests) | A public lab ([vizzini-lab](https://github.com/infrashift/vizzini-lab)) runs live models, real forges and real CI against ground truth | Claims are reproducible. Every release can be re-checked. |

## The capabilities

### 1. Reviews you can trust mechanically
- **The schema boundary:** every LLM response is validated against [`schema/review.cue`](schema/review.cue) with the CUE Go API before any caller sees it. This covers reviews, descriptions, improvements, plans and every command answer.
- **Repair or fail:** an invalid answer goes back to the model with the validation error, a bounded number of times. Vizzini never quietly passes malformed output on.
- **Anchoring:** each finding must sit on a line the diff actually changed, or it is repaired or dropped. The forge never gets a comment pointing at code that isn't in the change.
- **Large diffs:** a token-aware chunker splits big diffs and merges the results, so one large PR doesn't overflow a context window or lose files silently.

**Business value:** reviews are structured data you can build on: gates, dashboards, agents. They are not prose you hope parses. Developers never see broken or misplaced comments, which keeps their trust in the tool.

### 2. Reviews grounded in your code, your linters and your decisions
- **Lint-aware** ([guide](docs/src/content/docs/docs/guides/lint.md)): golangci-lint or SARIF reports (Roslyn, ESLint and others) are filtered to **changed lines** and given to the model. The model explains real diagnostics instead of guessing at them.
- **Repository context** ([guide](docs/src/content/docs/docs/guides/repository-context.md)): the definitions a change calls and the callers of what it changes are read from the checkout, so the review can judge effects beyond the diff.
- **Knowledge grounding** ([guide](docs/src/content/docs/docs/guides/knowledge.md)): an OKF wiki of your ADRs and design documents is matched to the files a change touches. Findings **cite the decision id** they enforce, and only relevant documents are sent.

**Business value:**
- **Fewer generic remarks:** findings tie to your linters and your own architecture rules, which reviewers can act on and auditors can trace.
- **Decisions get enforced:** a written decision stops being a document nobody reads, and becomes something reviews check against.

### 3. A bot that cannot authorize itself
`vizzini-bot` never talks to the forge directly. mrman does all forge work, and before each post, resolution or reply, mrman asks garmr to evaluate a **committed policy** ([ADR-0016](.okfw/decisions/), [ADR-0021](.okfw/decisions/)):
- **Base branch only:** the policy is read from the **base branch**, never from the PR. A pull request cannot loosen the rules it is judged by.
- **Deny first:** the starter policy lists no identities. The first run is denied, and its log names the identity to allow. Allowing it is itself a reviewed change.
- **Events and limits:** events are allow-listed (`comment`, `resolve`, `reply`; never `approve` by default). Each call has a comment cap, and a run id is required for traceability.
- **Audit:** every decision, allow or deny, is recorded with its policy digest.
- **No self-grant:** mrman's interactive grant needs a person at a terminal, so an agent cannot grant itself.

**Business value:** giving an AI write access to your review process becomes a governed, versioned and auditable act, the same as any other change to production configuration. Security and compliance teams can review and sign off on it.

### 4. Memory without a database
The bot's memory is the pull request's own threads. Each comment carries a hidden marker with the finding's fingerprint, its anchor and the head it reviewed ([`pkg/marker`](pkg/marker/marker.go)).
- **Incremental:** after the first run, only files changed since the last reviewed head are reviewed. Merges from the base branch are never re-reviewed.
- **No repeats:** a finding that matches an open thread is skipped. The match is by fingerprint, or by file, category and overlapping mapped lines, because models reword titles between runs.
- **Verifies fixes:** a thread is resolved only when its finding is gone **and** the code under it changed. A finding the model merely failed to repeat keeps its thread open. Files a push removes from the PR are reconciled too.
- **Stateless:** any CI runner can pick up where the last one left off, and there is nothing to migrate, back up or secure.

**Business value:**
- **Less noise:** repeated and stale comments are what make teams switch AI review off. This removes them.
- **Lower cost:** incremental review spends tokens only on what changed.
- **Nothing to run:** there is no service to operate.

### 5. Noise control that goes through review
- **Rules:** suppression rules live in `.vizzini/suppressions.yaml` and are read from the **base branch**. They match by path, category and title, and suppressed findings are still recorded.
- **Suggested rules:** when a person resolves a bot thread as *won't fix* or *by design*, the bot replies once with a ready-to-commit rule. The rule takes effect only after it is merged ([ADR-0020](.okfw/decisions/)).

**Business value:**
- **False positives disappear for good,** with a reviewed paper trail of what the team decided not to see, and why.
- **No self-silencing:** an author cannot quietly silence findings on their own change.

### 6. Disagreement without a deadlock
- **The dispute:** a developer, person or agent, can argue in the bot's thread and write `/vizzini reconsider`. The bot re-evaluates **that finding** against the code and the argument. The verdict is schema-validated ([ADR-0023](.okfw/decisions/)).
- **Withdrawn:** the bot resolves its own thread, with its reasons, and doesn't raise the finding again at that code.
- **Stands:** the thread stays open, and a **person decides**. An assertion without evidence does not withdraw a finding.
- **Agent guardrails:** a developer agent with its own account can reply, but cannot resolve the bot's threads, because mrman resolves only threads the token's own identity started. It also cannot approve or grant itself. vizzini-lab verifies these guardrails live.
- **Findings as data:** agents read findings as structured fields through `mrman review threads --findings`.

**Business value:**
- **Safe to automate fixes:** coding agents can work through review feedback while the reviewer stays independent, and a human still settles real disagreements.
- **False positives get retracted** in the thread, with the reasoning recorded.

### 7. A reviewer you can talk to
- **Commands:** `/vizzini explain`, `refactor`, `docstrings` and `reconsider` work in any file-anchored thread. Each is scoped to the selected lines and the thread's conversation, answered once, and attributed for cost. `--command-authors` and `--max-commands` bound them.
- **When answered:** on the next run in CI. With the optional, **experimental** webhook receiver, within seconds on Azure DevOps ([deploy/webhook](deploy/webhook/README.md)). It runs `vizzini-bot --commands-only`: a comment can trigger an answer, never a paid re-review.

**Business value:** reviewers get explanations and scoped refactors where the question arises, without leaving the PR or pasting code into a chat tool.

### 8. Deterministic risk and merge policy
- **The score** ([guide](docs/src/content/docs/docs/guides/risk.md)): a deterministic blast-radius score built from changed lines, added branching and per-path weights.
- **The model's role:** the model can **raise** the risk but never lower it.
- **Auto-approve:** `auto_approve` is true only when every rule in a CUE risk policy holds.
- **The gate:** garmr turns the JSON report into merge decisions, for example "deny on any Critical or Major finding" (VZ-001), or "deny on an Architecture finding citing an ADR" ([guide](docs/src/content/docs/docs/guides/garmr.md)).

**Business value:** routine changes can flow, risky ones get human attention, and the thresholds are reviewable code, not a vendor's black box.

### 9. Cost you can explain
- **Attribution:** every model call carries its run id, mode, PR key, iteration, agent and requester to LiteLLM, which writes them to its Postgres spend logs ([guide](docs/src/content/docs/docs/guides/cost-attribution.md)).
- **Reports:** SQL views report cost per request, per review run, per finding and across a PR's whole lifecycle, including bot reruns and commands.
- **No prompts stored:** prompts and diffs never go into the logs.
- **Budgets:** limits are set per LiteLLM virtual key.

**Business value:** AI review spend is explainable to finance per PR, per team and per person. It can be capped, and it doesn't create a second copy of your source code in a logging system.

### 10. Your models, your infrastructure, your forge
- **Any model:** through an OpenAI-compatible LiteLLM proxy (Claude, GPT, Gemini, local models). Vizzini needs no model-specific features. It validates and repairs whatever comes back.
- **Pure Go with no cgo:** a library ([`pkg/reviewer`](docs/src/content/docs/docs/guides/embedding.md)), a CLI (`pr-review`) and a bot (`vizzini-bot`). Small static binaries for UBI9 containers or microVMs. It can run fully **offline**, with a pre-seeded tokenizer cache.
- **Every major forge:** GitHub, GitLab (including **self-managed**) and Azure DevOps, through mrman.
- **CI templates** ([deploy/ci](deploy/ci/README.md)):
  - for GitHub Actions, GitLab CI and Azure Pipelines;
  - tools pinned and checked against checksums;
  - the policy read from the base branch;
  - each template verified live.
- **Embedded in a reviewer's terminal:** the mrman TUI shows Vizzini's findings on the diff, and a reviewer can promote any finding to a comment.

**Business value:**
- **Data sovereignty:** code and prompts stay on your network.
- **No lock-in:** switch models as prices and quality change.
- **Fits strict environments:** regulated and air-gapped teams can run it.
- **One tool across forges,** for organizations that use more than one.

### 11. Beyond review: describe, improve, plan, walk through
- **Describe:** PR titles and summaries.
- **Improve:** concrete code suggestions anchored to the diff.
- **Plan** ([guide](docs/src/content/docs/docs/guides/planning.md)): turns an issue into a schema-validated, phased implementation plan for a coding agent, grounded in the repository's file tree and knowledge base.
- **Walkthroughs** ([guide](docs/src/content/docs/docs/guides/walkthrough.md)): order a diff's files so reviewers read data models first, then logic, then UI.

**Business value:** one validated engine covers the PR lifecycle, from issue to plan to review to merge decision, so teams don't stitch tools together.

### 12. Verified in a public lab, not just unit tests
[vizzini-lab](https://github.com/infrashift/vizzini-lab) is a separate, public repository. It runs the whole chain (Vizzini, mrman, garmr, okf-wiki) against live Claude models, real forges and real CI, with ground truth. Results to date ([results/](https://github.com/infrashift/vizzini-lab/tree/main/results)):
- **Seeded fixtures:** in the recorded live runs, Claude Sonnet 5.5 and Opus 5.5 caught every planted C# and TypeScript defect. Every must-catch Security defect was found, with 100% category agreement, and the citation of the fixture's ADR was correct.
- **CI templates:** all three passed live: GitHub Actions 15/15, Azure Pipelines 13/13, GitLab CI 15/15.
- **Workflow suites:**
  - the unattended bot passed 47/47 with a live model, through nine legs;
  - the full fix, defend, dispute and settle workflow passed 26/26;
  - the webhook receiver passed 30/30.
- **Test plan:** 67 test cases a tester can run step by step, plus runbooks for every suite.

**Business value:** adopters can reproduce the claims on their own forge before trusting them, and every release is re-checked the same way.

## What is new here

The common building blocks are widely available: incremental reviews, chat commands, custom rules and self-hosting. [CodeRabbit](https://docs.coderabbit.ai/about/features) reviews incrementally and keeps chat-sourced [learnings](https://docs.coderabbit.ai/knowledge-base/learnings). [PR-Agent](https://github.com/The-PR-Agent/pr-agent) is self-hostable, with an incremental review and LiteLLM model choice ([overview](https://jimmysong.io/ai/pr-agent)). [Kodus](https://kodus.io/self-hosted-ai-code-review/) self-hosts and reads rule files.

What sets Vizzini apart is how it **governs** an AI reviewer. As far as we can find in these tools' public documentation:

1. **The reviewer's own write access is authorized by policy-as-code, per action.**
   - Each post, resolution and reply is evaluated against a committed garmr policy, with identity and event allow-lists, caps and audited decisions.
   - Other tools gate *merges* or configure *what to review*. We found none whose own comments must pass a committed authorization policy.
2. **Everything that can loosen the reviewer is read from the base branch.**
   - That covers the grant policy and the suppression rules.
   - By contrast, GitHub Copilot code review reads its custom instructions from the PR's **head** branch, so an author can rewrite them in the same PR ([Kodus's comparison](https://kodus.io/kodus-vs-github-copilot/), a vendor source).
   - Learnings in other tools are stored by the vendor. Vizzini's rules are files in your repository, changed only by reviewed merges.
3. **Fix verification, not just "don't repeat yourself."**
   - A thread is resolved only when the finding is gone **and** its code changed.
   - A finding the model merely failed to repeat stays open: the system allows for a model that isn't deterministic.
   - All of this state lives in the forge, with no database.
4. **A dispute protocol with a fixed arbiter.**
   - The bot may withdraw only its own finding (`/vizzini reconsider`), and a finding that stands always waits for a person.
   - Combined with mrman's own-thread resolve rule, a developer agent cannot close the reviewer's findings.
5. **Every model answer passes a schema boundary,** with repair. Reviews, commands and plans are validated data before anything posts or gates on them.
6. **Cost attribution down to the requester,** per run and across a PR's lifecycle, without logging prompts.
7. **A public, reproducible acceptance lab** that runs real forges and CI end to end. Third-party reviews of AI review tools note the lack of independent accuracy benchmarks, and Vizzini publishes its method and results.

**What we have not shown yet:** a head-to-head *quality* comparison. Precision on real PRs against PR-Agent and Kodus is planned ([ISSUE-022](.okfw/issues/)) but not done. Until then, Vizzini's claims are about governance, determinism and verification, not that it finds more bugs than other tools.

## Honest limits
- **Recall, not precision:** recall is measured on seeded defects, but precision on real PRs is not measured yet (ISSUE-022).
- **GitHub resolve permission:** on GitHub Actions, resolving threads needs `contents: write` for `GITHUB_TOKEN`. The template leaves it out by default, as a documented opt-in, because it also lets the token push.
- **The webhook receiver** is experimental and Azure DevOps-only. It needs a public HTTPS endpoint.
- **Language depth:** knowledge grounding by namespace and risk complexity are most precise for Go; other languages use documented estimates.
- **Model variance:** findings vary between models and runs. The memory rules allow for it, but a team should still calibrate rules and thresholds on its own code.

## Start here
- **Run it in CI:** [`deploy/ci/README.md`](deploy/ci/README.md). Copy a template and commit the grant policy; the first run tells you which identity to allow.
- **Use it locally:** [Getting started](docs/src/content/docs/docs/getting-started.md).
- **See it proven:** [vizzini-lab](https://github.com/infrashift/vizzini-lab), with its test plan in `.okfw/test-suites/`.
