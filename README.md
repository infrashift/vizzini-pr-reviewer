<p align="center">
  <img src="docs/src/assets/vizzini.jpeg" alt="The Go gopher as a curly-haired Renaissance schemer, arms spread over two goblets of wine, a wheel of cheese and a dagger on a clifftop above the sea." width="680">
</p>

<h1 align="center">Vizzini</h1>

<p align="center"><em>AI code review that never takes the model's word for it.</em><br>
<a href="https://infrashift.github.io/vizzini-pr-reviewer/">Documentation</a> · <a href="https://infrashift.github.io/vizzini-pr-reviewer/docs/project/roadmap/">Roadmap</a></p>

---

> So. You want to know when it ships.
>
> You think I would release a code reviewer that has not been reviewed? Me? I
> have counted your tokens, every one, and they fit. I have numbered your
> lines, the old ones *and* the new ones, so no model of mine can point at a
> line that does not exist. And when the model, so confident, so eloquent,
> hands me a finding with a severity of `"High"`, do I accept it? I unify it
> with my schema, and I send it back.
>
> Clearly I cannot choose the release date in front of *you*. But it is coming
> soon. A first tag before the CLI has been put through the gauntlet would be
> *inconceivable*.
>
> — Vizzini, on the subject of release dates

---

## What it is

Vizzini is a stateless, pure-Go AI code review engine. It is a library
(`pkg/reviewer`) built to embed in the [mrman](https://infrashift.github.io/mrman/)
terminal review tool, plus a static `pr-review` CLI for CI, UBI9 containers and
offline QEMU microVMs.

- **Every answer is checked.** Model output is unified with closed CUE
  definitions (`schema/review.cue`) and anchored to lines that actually appear
  in the diff. Anything that fails goes back to the model with the exact
  error, and you never see it.
- **Token-aware chunking.** Diffs are packed into chunks within an exact
  tiktoken budget and split only between hunks, never inside one.
- **Any model, one proxy.** Requests use the OpenAI chat API in JSON mode
  against your LiteLLM proxy, with backoff on transient failures.
- **Small, static, offline.** `CGO_ENABLED=0`, a short pinned dependency
  list, and tokenizer encodings pinned by checksum, so token counting never
  needs the network.

## Status

**Not released yet.** The engine, the Go API and the `pr-review` CLI build
and pass their tests today. That includes the severity policy gate
(`--fail-on`), lint-aware reviews from golangci-lint or any SARIF tool (`--lint-json`), risk scoring with auto-approve (also without a model: `--mode risk` and the
embeddable `pkg/risk`), dependency-ordered walkthroughs, and design-decision citations
from an OKF knowledge base (`--knowledge`), `/explain` `/refactor`
`/docstrings` slash commands, issue-to-plan handoff for coding agents
(`--mode plan`), and JSON reports ready for the
[garmr](https://github.com/infrashift/garmr) policy agent (`policies/garmr`). There is
also `vizzini-bot`, which reviews every pull request from CI with no person
involved. It posts through mrman only when a committed policy allows it, remembers
what it said, re-reviews only what changed, answers `/vizzini` commands in PR
threads, and honors your suppression rules. Every milestone on the
[roadmap](https://infrashift.github.io/vizzini-pr-reviewer/docs/project/roadmap/)
is implemented and has been run against live Claude models on Azure DevOps.
What's left before a tag is a release pipeline. Expect sharp edges.

## Try it from source

You need Go 1.25+, the `cue` CLI, `golangci-lint` v2, and an OpenAI-compatible
endpoint such as a LiteLLM proxy.

```sh
git clone https://github.com/infrashift/vizzini-pr-reviewer
cd vizzini-pr-reviewer
make test build    # first run seeds the tokenizer cache (about 5.3 MB, checksum-pinned)

export VIZZINI_LITELLM_URL=http://litellm:4000/v1
export VIZZINI_MODEL=<model name in your LiteLLM config>
export VIZZINI_API_KEY=<proxy key>
export TIKTOKEN_CACHE_DIR=$PWD/.cache/tiktoken

git diff main...HEAD | ./bin/pr-review --mode review
git diff main...HEAD | ./bin/pr-review --mode describe --format json
```

## Contributing

Work proceeds one phase at a time (see [`PLAN.md`](PLAN.md)). Each change must
pass `make test && make vet-cue && make lint`. Design decisions, specs and
issues live in the project's OKF knowledge base under [`.okfw/`](.okfw/). Read
the [contributing guide](https://infrashift.github.io/vizzini-pr-reviewer/docs/project/contributing/)
before your first change.
