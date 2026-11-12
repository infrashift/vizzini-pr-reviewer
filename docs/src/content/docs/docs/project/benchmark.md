---
title: "Benchmark: Vizzini vs PR-Agent"
description: A head-to-head comparison of Vizzini and PR-Agent on the same pull requests, models and proxy, with the method, versions, data and caveats, including where Vizzini does worse.
---

In November 2026 we compared Vizzini with [PR-Agent](https://github.com/the-pr-agent/pr-agent), the most widely deployed open-source AI reviewer. Both tools reviewed the same pull requests, with the same models, through the same proxy. We wanted to know where Vizzini helps and where it does not. Everything needed to check or re-run the comparison is public in [vizzini-lab](https://github.com/infrashift/vizzini-lab).

:::note[In short]
On 26 real pull requests that introduced bugs:
- **Finding the bugs:** the two tools find about the same share. The current Vizzini found 72% of the labelled defects with Claude Sonnet 5.5, against PR-Agent's 62%. With Claude Opus 5.5 it found 62%, against PR-Agent's 66%. Neither difference is statistically clear.
- **Finding only real problems:** in the first comparison, PR-Agent was clearly more precise. Only about half of Vizzini's findings were real problems, against 63% to 74% of PR-Agent's.
- **After the fix:** we then changed Vizzini to cut that noise (a [noise floor](#after-issue-032-a-noise-floor-and-a-category-rule) and a clearer category rule). 68% to 69% of its findings are now real problems, and it lost no recall. The precision gap to PR-Agent is no longer statistically clear: 69% against 63% with Sonnet, and 68% against 74% with Opus.
- **Time and cost:** Vizzini took about a third of the time and half the model cost.
- **Lint output and knowledge bases:** giving Vizzini lint output or a project knowledge base made no measurable difference.
:::

## What was compared

| | Vizzini | PR-Agent |
|---|---|---|
| Version | commit [`b002fad`](https://github.com/infrashift/vizzini-pr-reviewer/commit/b002fad154a3ea2b7ab077a98e2616de2122de9e) for the first comparison; [`38d9959`](https://github.com/infrashift/vizzini-pr-reviewer/commit/38d9959) after [ISSUE-032](#after-issue-032-a-noise-floor-and-a-category-rule) (no release yet) | **0.49.0**, the published image `pragent/pr-agent:0.49.0@sha256:10a8893d…`, the newest release at least two weeks old |
| Commands | `pr-review --mode review` | `review` and `improve`, its two commands that find problems, counted together |
| Configuration | default | default (`review` reports at most 3 key issues) |
| Code it sees | the diff, plus definitions and callers from the repository (`--repo`) | the diff, plus the changed files in full (its `local` git provider on the same checkout) |

- **Models:** both tools used `claude-sonnet-5-5` and `claude-opus-5-5`, through one LiteLLM v1.99.4 proxy. Prices were $2 in / $10 out per million tokens for Sonnet, and $4 / $20 for Opus.
- **Inputs:** in the first round, neither tool got lint output or a knowledge base. Rounds 2 and 3 added these for Vizzini (see [What lint and knowledge added](#what-lint-and-knowledge-added)).

## The test set

34 cases, every one pinned to exact commits, in [`bench/cases.json`](https://github.com/infrashift/vizzini-lab/blob/main/bench/cases.json):

- **26 real bug-introducing changes** from 20 permissively licensed projects (Go 7, Python 6, JavaScript 5, C# 4, TypeScript 2, Rust 2).
  - **How they were chosen:** for each, a later commit fixed a bug and its own message or PR names this change as the cause. We used no guesswork such as SZZ.
  - **The labels:** the defect lines (29 defects in all) are the lines of the change that the fix corrects. They were verified against the code.
  - **Dates:** 15 of the changes were made after the models' knowledge cutoff (July 2026 or later), so the models cannot have seen their fixes.
- **5 clean changes** of similar size, which no later commit changed or blamed. They measure noise.
- **3 seeded fixtures** we wrote ourselves:
  - a C# service with 9 planted defects;
  - a TypeScript service with 10 planted defects;
  - a small Go module whose 3 defects are visible only with code outside the diff, plus one decoy that only looks wrong.

  Because we wrote them, the fixtures are reported separately and are not the headline.

<details>
<summary>All 31 real cases (* = introduced after the models' knowledge cutoff)</summary>

| Case | Project | Language | Introduced | License | Kind | Labelled defects |
|---|---|---|---|---|---|---|
| `cs-aspnetcore-rewrite-backslash` | [dotnet/aspnetcore](https://github.com/dotnet/aspnetcore/commit/0563bffcb76c5be3d812e275df30c766b17541fa) | C# | 2026-06-24 | MIT | buggy | 1 |
| `cs-avalonia-popup-pointer-resize` | [AvaloniaUI/Avalonia](https://github.com/AvaloniaUI/Avalonia/commit/cc2aeb295f36939c3c459e3efcd6d43b1753ff27) | C# | 2026-10-05 * | MIT | buggy | 1 |
| `cs-avalonia-vsp-anchor-at-end` | [AvaloniaUI/Avalonia](https://github.com/AvaloniaUI/Avalonia/commit/7f8dd8e6bd5557fb30f935c261db05acd38821b5) | C# | 2026-08-24 * | MIT | buggy | 1 |
| `cs-efcore-executedelete-commandsource` | [dotnet/efcore](https://github.com/dotnet/efcore/commit/a0443c1460e33f76094a30dc5ae0524743ed02f5) | C# | 2024-09-27 | MIT | buggy | 1 |
| `go-cilium-mapstate-prefix` | [cilium/cilium](https://github.com/cilium/cilium/commit/17838ff0876fb2e8c151a518dc2d923c9ca76413) | Go | 2026-06-05 | Apache-2.0 | buggy | 1 |
| `go-gitea-diff-linecount-shadow` | [go-gitea/gitea](https://github.com/go-gitea/gitea/commit/ae176cd6493480f27b138fb77701d6ac251ebe2c) | Go | 2026-07-19 * | MIT | buggy | 1 |
| `go-gitea-runner-bulk-empty-ids` | [go-gitea/gitea](https://github.com/go-gitea/gitea/commit/dd59c6848660ffa31db0db8c14c2a610076a7a8b) | Go | 2026-05-29 | MIT | buggy | 1 |
| `go-gitea-tail-diffline-empty` | [go-gitea/gitea](https://github.com/go-gitea/gitea/commit/d87d26d7352cbd76f5e5139c4f94504fbd465bf8) | Go | 2026-07-30 * | MIT | buggy | 1 |
| `go-otelcol-hostwrapper-capabilities` | [open-telemetry/opentelemetry-collector](https://github.com/open-telemetry/opentelemetry-collector/commit/2ab65ae99cb1d65b93f460a95c26d50c3485d735) | Go | 2026-07-20 * | Apache-2.0 | buggy | 1 |
| `go-rclone-shade-complete-eof` | [rclone/rclone](https://github.com/rclone/rclone/commit/a4972de505afc1cbef45a3aa338a4df0ad905b86) | Go | 2026-07-20 * | MIT | buggy | 1 |
| `go-tailscale-socks5-halfclose` | [tailscale/tailscale](https://github.com/tailscale/tailscale/commit/027e249fcf664458a5dae7fa71b418b80750f9c9) | Go | 2026-09-17 * | BSD-3-Clause | buggy | 2 |
| `js-fastify-forceclose-idle` | [fastify/fastify](https://github.com/fastify/fastify/commit/c2ba18c17769dacce61472ec0397560b3b4eebde) | JavaScript | 2026-04-19 | MIT | buggy | 1 |
| `js-node-child-process-env-reassigned` | [nodejs/node](https://github.com/nodejs/node/commit/bd83a5cd7a80c3b4b1378fd66c700bff9752847f) | JavaScript | 2026-09-30 * | MIT | buggy | 1 |
| `js-node-stream-asynciter-undefined` | [nodejs/node](https://github.com/nodejs/node/commit/4551732bf1137bdc1e49d26ab6e26bec9c065e8d) | JavaScript | 2026-08-14 * | MIT | buggy | 2 |
| `js-prettier-markdown-cj-null-deref` | [prettier/prettier](https://github.com/prettier/prettier/commit/5927216227411bc2cdaf30d50559e0a475ab4832) | JavaScript | 2026-10-04 * | MIT | buggy | 1 |
| `js-undici-h2-maxconcurrentstreams-validation` | [nodejs/undici](https://github.com/nodejs/undici/commit/ab0ecfd6cf536ffda72c16ba96eb67f661a1f401) | JavaScript | 2026-07-27 * | MIT | buggy | 1 |
| `py-django-nonatomic-async` | [django/django](https://github.com/django/django/commit/bdbda29c3e126754c3ae04ceb5c5d35d49aae01c) | Python | 2026-05-08 | BSD-3-Clause | buggy | 1 |
| `py-django-verbatim-lexer` | [django/django](https://github.com/django/django/commit/1a9fba7a6c2a0acead19bbb45c25ceb0fdcca7c8) | Python | 2026-10-13 * | BSD-3-Clause | buggy | 1 |
| `py-django-yaml-sortkeys` | [django/django](https://github.com/django/django/commit/7142667340f78403cbbcaa8f33703c494282b7b7) | Python | 2026-04-04 | BSD-3-Clause | buggy | 1 |
| `py-numpy-linspace-subclass` | [numpy/numpy](https://github.com/numpy/numpy/commit/a2dea9776752caf57bb72ca4158c51b898009e62) | Python | 2026-07-06 * | BSD-3-Clause | buggy | 1 |
| `py-pandas-agg-func-names-truthiness` | [pandas-dev/pandas](https://github.com/pandas-dev/pandas/commit/b8997bc65807a25f9c7d866668e74e142419b6df) | Python | 2026-09-28 * | BSD-3-Clause | buggy | 1 |
| `py-pytest-initial-conftest-testdirs` | [pytest-dev/pytest](https://github.com/pytest-dev/pytest/commit/b665afc848a83f7498b9a52c467ec255bbd208a1) | Python | 2026-03-22 | MIT | buggy | 1 |
| `rs-rust-analyzer-fetch-semantic` | [rust-lang/rust-analyzer](https://github.com/rust-lang/rust-analyzer/commit/c2b0c4b6342170bd109efcd317527450f3150ad2) | Rust | 2026-05-04 | MIT OR Apache-2.0 | buggy | 1 |
| `rs-tokio-pidfd-child-leak` | [tokio-rs/tokio](https://github.com/tokio-rs/tokio/commit/e4f9bcb5775a8cbbc848aedea3ad49aa60dd1dae) | Rust | 2024-01-11 | MIT | buggy | 1 |
| `ts-react-router-manifest-paths-comma` | [remix-run/react-router](https://github.com/remix-run/react-router/commit/49815ae75af76b522dee09d5991697bd7242c27b) | TypeScript | 2025-09-17 | MIT | buggy | 2 |
| `ts-vue-vapor-slot-close-lazy` | [vuejs/core](https://github.com/vuejs/core/commit/92971551dc81bbff6db3f3e14d2a83458ac4e0de) | TypeScript | 2026-10-27 * | MIT | buggy | 1 |
| `cs-avalonia-commandbar-overflow-clean` | [AvaloniaUI/Avalonia](https://github.com/AvaloniaUI/Avalonia/commit/c00a2f23750a2a5e649073e663952482bf2babb7) | C# | 2026-07-10 * | MIT | clean | 0 |
| `go-helm-ssa-conflict-retry-clean` | [helm/helm](https://github.com/helm/helm/commit/aa1ae3a36017156817e89e59eee0e88af9fc6365) | Go | 2026-07-15 * | Apache-2.0 | clean | 0 |
| `go-tailscale-controld-doh-clean` | [tailscale/tailscale](https://github.com/tailscale/tailscale/commit/d2af6a4d39adbca71792cf0f89119e513e111e1c) | Go | 2026-07-17 * | BSD-3-Clause | clean | 0 |
| `js-undici-frozen-globalthis-clean` | [nodejs/undici](https://github.com/nodejs/undici/commit/a0922b0b6b5db878881017abb6fca3bbcdea555a) | JavaScript | 2026-07-22 * | MIT | clean | 0 |
| `py-django-cacheddb-delete-logging-clean` | [django/django](https://github.com/django/django/commit/a486b48d4ea0d34babe58cae215b2123748aacbd) | Python | 2026-07-09 * | BSD-3-Clause | clean | 0 |
</details>

## How it was scored

Every finding from every tool, model and round was **judged blind**: 1,350 distinct findings. The judges never saw which tool wrote a finding, and read the diff and the repository to decide each one.

| Verdict | Meaning |
|---|---|
| defect | It identifies a labelled defect: it is in that defect's code and describes the actual failure. |
| valid | A real, correct problem in the change that a reviewer would want raised. |
| nit | Correct but trivial: style, naming, comments, unreachable defensive checks. |
| invalid | Wrong about the code, speculative, or not about the change. |

- **Recall:** the share of labelled defects that some finding identifies.
- **Strict precision:** (defect + valid) / all findings.
- **Lenient precision:** strict precision, with nits also counted as good.
- **The judges:** nine Claude Opus 5.5 agents applied [the rubric](https://github.com/infrashift/vizzini-lab/blob/main/bench/RUBRIC.md).
- **Agreement:** a second, independent judge re-judged a random sample of 150 findings and agreed on **91%** of them (Cohen's κ 0.90). The two judges never disagreed on whether a finding was a defect or valid; every disagreement was between nit and invalid. So strict precision and recall hold up across judges, and lenient precision is softer.
- **Our own review:** we read every call the judges were unsure about, and made two changes.
  - **One strict rule.** A finding counts as finding a defect only if it is in that defect's own code and describes the failure itself, not just its cause. This first removed defect credit from 26 findings, 18 of them Vizzini's and 8 PR-Agent's.
  - **One labelled defect removed,** because the fix never changed its code. Its 9 findings, 6 of them PR-Agent's, now count as valid.
- **One uniform pass, after the first version of this page.** The judges of later runs read the strict rule a little differently in a few cases. So we applied it once more, in one pass, to every finding with defect credit on the real pull requests: 388 findings, from every run and both tools.
  - **What changed:** 26 lost their credit and became valid or invalid; 14 of these were Vizzini's in the first comparison, 8 PR-Agent's, and 4 from the later Vizzini runs.
  - **Effect:** recall fell for both tools, by 3 to 10 points against the first version of this page, and no conclusion changed. The numbers on this page include the pass.
  - **Where to check:** every changed verdict keeps the earlier verdict and the reason, in [`labels.json`](https://github.com/infrashift/vizzini-lab/blob/main/bench/labels.json).
- **Intervals:** each configuration ran once. The brackets below are 95% bootstrap intervals over cases. They show how much the numbers depend on which cases were drawn, not run-to-run variation.

## Results on real pull requests

| | Defects found | Recall | Strict precision | Lenient precision | Findings per PR |
|---|---|---|---|---|---|
| **Vizzini `38d9959`** (after ISSUE-032), Sonnet 5.5 | 21 / 29 | **72%** [56%, 88%] | **69%** [55%, 82%] | 87% | 2.2 |
| Vizzini `b002fad` (first comparison), Sonnet 5.5 | 20 / 29 | 69% [52%, 86%] | 51% [37%, 65%] | 78% | 2.7 |
| **PR-Agent**, Sonnet 5.5 | 18 / 29 | 62% [45%, 78%] | 63% [47%, 78%] | 80% | 2.7 |
| **Vizzini `38d9959`** (after ISSUE-032), Opus 5.5 | 18 / 29 | 62% [45%, 79%] | 68% [56%, 80%] | 87% | 2.5 |
| Vizzini `b002fad` (first comparison), Opus 5.5 | 17 / 29 | 59% [41%, 77%] | 50% [37%, 62%] | 82% | 3.5 |
| **PR-Agent**, Opus 5.5 | 19 / 29 | **66%** [48%, 83%] | **74%** [61%, 87%] | 83% | 2.8 |

Paired on the same cases, these are the differences that hold with 95% confidence:

- **ISSUE-032 raised Vizzini's precision:** by 18 points with Sonnet [9, 28] and 19 points with Opus [11, 26], with no loss of recall (+3 points with each model).
- **First comparison:** PR-Agent was more precise than Vizzini `b002fad` with Opus, by 25 points [13, 36]. With Sonnet the gap was 12 points [0, 25], right at the edge.
- **Now:** no precision difference holds between Vizzini `38d9959` and PR-Agent. Vizzini is 6 points ahead with Sonnet [−9, +19], and 6 points behind with Opus [−18, +5].
- **No recall difference holds** in either comparison. Vizzini `38d9959` is 10 points ahead with Sonnet [0, +22], and 3 points behind with Opus [−19, +11].

Other observations:

- **Before and after the knowledge cutoff (first comparison):** on the 17 defects introduced after the cutoff, Vizzini found 9 (Sonnet) and 8 (Opus), and so did PR-Agent. On the 12 older defects, Vizzini found 11 (Sonnet) and 9 (Opus), and PR-Agent 9 (Sonnet) and 11 (Opus). Vizzini `38d9959` found 11 and 9 of the newer defects.
- **Defects no one found:** no tool and no model found the defects in the [rclone](https://github.com/rclone/rclone) and [pandas](https://github.com/pandas-dev/pandas) cases. One needs knowledge of how a storage API answers. The other is a truthiness check on a list of function names.
- **Clean changes (5):** both tools raised one to three findings per change, mostly nits or valid. Vizzini `b002fad` with Opus raised the most, 12 in all, 3 of them invalid; Vizzini `38d9959` raised 9 with each model. PR-Agent raised 6 to 8 in all, 1 to 2 of them invalid.

## After ISSUE-032: a noise floor and a category rule

The first comparison showed Vizzini's main weakness, so we worked on it ([ISSUE-032](https://github.com/infrashift/vizzini-pr-reviewer/tree/main/.okfw/issues)). The judged findings showed where its noise was. On real pull requests:
- **Trivial findings:** 6% were real problems, and 74% were nits.
- **Minor Performance findings:** about a fifth were real problems.
- **Major findings:** 81% were real problems.

Two changes followed, in commit `38d9959`:

- **A default noise floor.** Trivial findings and Minor Performance findings leave the review's findings for a `filtered` list in the JSON report. They are not shown or posted, but nothing is lost: `--fail-on` and auto-approval still see them, and `--all-findings` shows them ([CLI reference](../../reference/cli/#the-noise-floor)).
- **A category rule in the prompt.** A behaviour bug is Logic, never Maintainability, even when its cause looks structural.

How we chose and checked them:
- **Measured on fresh reviews.** Both changes were measured on new runs of all 34 cases, judged blind like the first ones, not only on the data they were chosen from.
- **The floor's own effect.** Within the new run, the floor alone added 11 to 13 points of precision and lost no defect. The category rule accounts for the rest of the gain.
- **A wider floor we did not ship.** It also filtered Minor Maintainability findings, which were as noisy. It added more precision, but in one run it hid 2 of 29 real defects: bugs the model had filed under Maintainability. That is why the default keeps those findings, and why the category rule exists. Teams that want less noise can add the rule through `Config.NoiseFloor`.

Vizzini `38d9959` still finds every planted defect in the seeded fixtures (9/9, 10/10, 3/3). Its time and cost are unchanged.

## Results on the seeded fixtures

| | C# (9 defects) | TypeScript (10) | Context-dependent Go (3) | Decoy flagged |
|---|---|---|---|---|
| Vizzini, Sonnet / Opus | 9 / 9 | 10 / 10 | 3 / 3 | yes |
| Vizzini without `--repo`, Sonnet / Opus | 9 / 9 | 10 / 9 | 2 / 2 | no / yes |
| PR-Agent, Sonnet | 6 | 4 | 2 | no |
| PR-Agent, Opus | 9 | 5 | 2 | yes |

Vizzini finds every planted defect, but this mostly reflects how the fixtures were built.
- **The three-issue cap:** the fixtures pack up to 10 defects into one change, and PR-Agent's `review` reports at most 3 key issues by default.
- **Real changes are different:** most real changes hold one bug, and there the gap disappears (above).
- **The context fixture:** Vizzini's repository context found all three defects that need code outside the diff, against two without it. It still flagged the decoy as a possible nil dereference, although the context shows the value is never nil.

## What lint and knowledge added

We ran Vizzini twice more on the same cases.

- **Round 2, with lint output** (`--lint-json`):
  - **Linters:** golangci-lint v2.15.0, ruff 0.17.2 or ESLint 9, with one neutral configuration for every project. Nothing was built.
  - **Not linted:** the C# and Rust real cases, because their analyzers need a build.
  - **Result:** no difference held for recall or precision with either model.
- **Round 3, with a knowledge base** (`--knowledge`):
  - **How the wikis were written:** an agent wrote a small OKF wiki of conventions for each project, using only the project's own documentation as it stood before the change. It never saw the defects. The wikis are in [`bench/knowledge`](https://github.com/infrashift/vizzini-lab/tree/main/bench/knowledge).
  - **PR-Agent got the same text** as its `extra_instructions`.
  - **For Vizzini:** no measurable difference.
  - **For PR-Agent with Opus:** precision fell 7 points [1, 15], and cost rose by about 60%.

So on these cases, neither lint output nor documented conventions helped the model find real bugs. Lint diagnostics on changed lines were few: 52 across 54 runs. The projects' documented conventions also say little about the kinds of bug in this set. Teams whose rules are specific and often broken may see more benefit. This benchmark does not show that.

Repository context (`--repo`) gave a small precision gain with Opus: 6 points [0, 12] over the diff alone.

## Time and cost

| | Median time per review | Model cost per review |
|---|---|---|
| Vizzini, Sonnet / Opus | 17 s / 21 s | $0.040 / $0.079 |
| PR-Agent `review` + `improve`, Sonnet / Opus | 54 s / 58 s | $0.080 / $0.160 |
| PR-Agent `review` alone, Sonnet / Opus | 25 s / 26 s | $0.032 / $0.066 |

These are wall times through one local proxy, three reviews at a time. Absolute times will differ on your setup; the ratios are the useful part.

## Strengths and weaknesses

**Vizzini**
- ✅ About as good at finding real bugs as PR-Agent's two commands together, at about a third of the time and half the cost.
- ✅ No cap on findings: when a change holds many problems, it reports them all.
- ✅ Repository context finds bugs that need code outside the diff.
- ✅ Every finding is schema-checked and anchored to a changed line.
- ⚠️ **Precision was its weak point.** Version `b002fad` was noisier: about half its findings were not real problems, against a third or fewer of PR-Agent's. Version `38d9959` brought that to about a third, level with PR-Agent within the margin. With Opus, PR-Agent is still a little ahead, by 6 points, but that difference is within the margin of error.
- ❌ It still raised a few more findings on clean changes than PR-Agent did, mostly nits.
- ❌ Lint output and a knowledge base did not improve its results here.

**PR-Agent**
- ✅ **Precise out of the box.** It was clearly more precise than Vizzini `b002fad` with Opus, and its precision is still level with or above Vizzini `38d9959`.
- ✅ Its `security_concerns` section adds file-level security notes beyond the scored findings.
- ❌ Slower and more expensive, because it runs two commands.
- ❌ `review`'s default cap of 3 key issues misses defects when a change holds many.
- ❌ Extra knowledge (`extra_instructions`) cost more and made it less precise with Opus.

## Limitations

- **Small sample.** 29 real defects. Only large differences can be told apart from chance, and most of the intervals above are wide.
- **Few runs per configuration.** Model output varies from run to run. Two Vizzini runs without the floor gave a sense of it: with Sonnet they found 22 and 21 of the 29 defects, with Opus 19 and 18 (before the uniform pass). PR-Agent ran once.
- **Claude judged Claude.** The judge model is also one of the models under test. Judging was blind to the tool, but each tool's writing style could still give it away. The published rubric, the reasons and the second-judge agreement are there so you can check.
- **Defaults only.** PR-Agent has many settings, and a tuned configuration might do better; so might Vizzini with other settings. We compared the tools as most people first deploy them.
- **What "a bug" means here.** A real case counts only the bug that a later fix named. Other true findings count as valid, not as hits.
- **Memorization.** 11 real cases predate the models' knowledge cutoff. The split above shows no clear advantage on those.
- **We built Vizzini.** That is why we publish the cases, the labels with reasons, the judges' notes and the scripts, and why we report where Vizzini does worse.

## Reproduce it

The benchmark runs in [vizzini-lab](https://github.com/infrashift/vizzini-lab). You need podman, an Anthropic key and about $37 of model time for all three rounds.

```sh
make build litellm-up
make bench CASES=all                 # round 1: both tools, both models
uat/bench-lint.sh "$BENCH_RUN"       # lint for round 2
BENCH_RUN=... uat/bench.sh --round 2
BENCH_RUN=... uat/bench.sh --round 3
make bench-score BENCH_RUN=...       # scores with bench/labels.json
```

- **Method:** lab [ADR-0013](https://github.com/infrashift/vizzini-lab/tree/main/.okfw/decisions).
- **Judges' notes and our review:** [`bench/judging/NOTES.md`](https://github.com/infrashift/vizzini-lab/blob/main/bench/judging/NOTES.md).
- **Full results:** [`results/2026-11-12-bench`](https://github.com/infrashift/vizzini-lab/tree/main/results/2026-11-12-bench), with per-case tables, intervals and every finding with its verdict.
- **New findings** (for example from a new tool version) have no verdict. `bench.py` lists them in `to-label.jsonl` for judging.
- **Comparing versions:** `uat/bench.sh --tools vizzini` runs only Vizzini, and `uat/bench_compare.py` compares two runs case by case, with intervals. That is how `38d9959` was measured against `b002fad`.
