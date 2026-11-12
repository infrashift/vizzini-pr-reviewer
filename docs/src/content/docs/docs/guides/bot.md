---
title: Unattended PR Review (vizzini-bot)
description: Review every pull request from CI and post the findings as comments, with no person at a terminal, only when a committed garmr policy allows it.
---

`vizzini-bot` is the CI entry point. It runs in the pipeline's checkout,
reviews `git diff base...head` with the Vizzini library, and posts each
finding as a comment thread on the pull request. It never talks to the forge
itself: [mrman](https://github.com/infrashift/mrman) does all forge work,
and mrman posts only when [garmr](../garmr/) allows it under a policy
committed to your repository.

```sh
vizzini-bot \
  --pr-url "$PR_URL" --base "origin/$TARGET_BRANCH" --head HEAD \
  --grant-policy policies/garmr/mrman \
  --knowledge .okfw --lint-json build.sarif \
  --pr "ado:dev.azure.com/org/project/repo#$PR_ID" --requester "$PR_AUTHOR" \
  --out vizzini-review.json
```

## What happens

1. **Remember.** `mrman pr URL --json` opens the PR headlessly, and
   `mrman review threads` lists what the bot said in earlier runs (see
   [thread memory](#thread-memory)).
2. **Review.** On the first run, `git diff base...head` in `--repo`
   (default `.`) is reviewed. After that, only the files changed since the
   last reviewed head are reviewed (see
   [incremental review](#incremental-review)), and nothing at all when the
   head has not moved.
   - The checkout supplies [repository context](../repository-context/), and
     you can add a [knowledge base](../knowledge/) and
     [linter reports](../lint/).
   - Every request carries [attribution](../cost-attribution/): agent
     `vizzini-bot`, the head commit as the iteration, and the run ID.
   - Findings an earlier thread already raises are skipped.
3. **Draft.** `mrman review add` drafts one comment per new finding. Each
   comment ends with a hidden marker,
   `<!-- vizzini:fp=v1:… run=… head=… lines=…-… cat=… -->`. It records the
   finding's fingerprint (file, category and normalized title) and where it
   was anchored, at which commit.
4. **Authorize and post.**
   `mrman review submit --event comment --grant-policy DIR` asks garmr
   whether this event, by this identity, may be posted.
   - The identity is what the forge reports for the token, not anything the
     bot claims.
   - On allow, mrman posts. On deny, or if the policy cannot be evaluated,
     nothing is posted.
   - Every decision goes to stderr and to mrman's audit log, with the policy
     digest and the run ID.

5. **Resolve.** Threads whose finding is gone, and whose code changed since
   they were posted, are resolved with a short reply through
   `mrman review resolve`, under the same policy (event `resolve`).
6. **Answer.** `/vizzini` commands people wrote in the PR's threads are
   answered in those threads through `mrman review reply` (event `reply`).
   See [commands in threads](#commands-in-threads).

stdout is one JSON object for the run:

```json
{"run_id":"…","head":"…","mode":"incremental","since":"…","reviewed_files":["src/a.cs"],
 "findings":2,"posted":1,"already_posted":["v1:…"],"resolved":["197"],"kept_open":[],
 "carried":["195"],"suppressed":["legacy-frozen src/Legacy/ReportFormatter.cs:7"],"suggested":[],
 "answered":["4021"],"command_failures":[],"submit":{…},"resolve":{…}}
```

`mode` is `full`, `incremental` or `unchanged` (no review was needed).

| exit | meaning |
|---|---|
| 0 | done: posted, resolved, or there was nothing to do |
| 1 | an error (git, review, mrman) |
| 3 | the grant policy denied posting, resolving or replying |

mrman calls that fail with a network error are retried, up to three
attempts. A submit that failed that way keeps its unposted comments for the
next attempt.

## Incremental review

The bot does not re-review the whole pull request on every push.
- **Last reviewed head:** every posted finding's marker records the head it
  was posted at. The newest of those that is an ancestor of the current
  head is the last reviewed head.
- **What is reviewed:** the pull request's own diff (`base...head`),
  limited to the files that changed since that head. Each file is reviewed
  with its whole PR diff as context.
- **Base merges are left out:** changes merged in from the base branch are
  not part of `base...head`, so they are never reviewed.
- **No review needed:** when the head has not moved, or a push only merges
  the base branch, no model call is made (`mode: "unchanged"`).
- **Carried threads:** open threads on files that were not reviewed are
  carried as they are (`carried`), neither resolved nor kept open.
- **Full review instead:** with `--full`, or with no usable earlier head
  (the first run, a rebased or force-pushed branch, a shallow clone), the
  whole pull request is reviewed.

Every comment the bot posts (a finding, a resolution, an answer) records the
head its run reviewed. A run that posts nothing at all leaves no trace, so
the next run reviews from the last head that did post: more than needed,
never less. On the UAT's C#
fixture, an incremental review of a one-file push cost about 1/7 of a full
review.

## Commands in threads

Anyone who can comment on the pull request can ask the bot something. To do
that, write a command on a line of its own in any file-anchored thread,
including the bot's own:

```text
/vizzini explain [FILE:START-END] [question]
/vizzini refactor [FILE:START-END] [what to change]
/vizzini docstrings [FILE] [note]
/vizzini reconsider [why the finding is wrong]
```

`/vizzini reconsider` is a structured disagreement, valid only in a thread
the bot opened for a finding. The bot re-evaluates **that finding** against
the code and the thread's arguments:
- **Withdrawn:** the bot resolves its own thread, with its reasons as the
  resolution's reply. A later run at the same code does not post the
  finding again.
- **Stands:** the bot replies with its reasons and leaves the thread open,
  so a person decides. An argument without evidence does not withdraw a
  finding, and the model never settles a dispute alone.

How the bot handles a command:
- **Target:** without one, the command is about the thread's own lines (or
  its file, for `/docstrings`).
- **Context:** the thread's earlier comments travel with the command as
  the conversation.
- **When it's answered:** the next bot run answers it in the thread, even
  if no review is needed. Each command is answered once: the reply carries
  a hidden `reply-to` marker. On Azure DevOps, the optional
  [webhook receiver](../webhook/) (experimental)
  answers it as soon as it is written, with `--commands-only`.
- **Problems:**
  - A command the bot cannot understand, or one whose target is outside
    the PR's diff, gets one reply explaining how to write it.
  - A failure to reach the model gets no reply, so the next run tries
    again.
- **Limits:** `--max-commands` (default 5) bounds the commands answered per
  run. `--command-authors alice,bob` limits who may run them; by default,
  anyone who can comment on the PR can. Command requests are attributed
  like the review, so their cost is in the same spend logs.

## Suppressing findings

When a team decides it does not want to hear about a kind of finding again,
it records that in `.vizzini/suppressions.yaml`:

```yaml
suppressions:
  - id: legacy-frozen
    reason: The legacy formatter is frozen until its rewrite (ADO #1234).
    paths: ["src/Legacy/**"]
    categories: [Maintainability]
  - id: admin-tool-sql
    reason: Only the internal admin tool calls this; reviewed in ADR-0007.
    paths: ["src/Admin/**"]
    title: sql injection
    expires: "2027-06-30"
```

**Writing a rule.** A rule needs an `id` and a `reason`, plus at least one
of:
- `fingerprint`: one exact finding;
- `paths`: globs;
- `title`: a case-insensitive substring.

`categories` and `expires` narrow a rule further. The file is validated
against `#Suppressions`, and an invalid file fails the run instead of being
ignored.

**What a rule does.** A matching finding is reported under `suppressed`
with the rule's id. It is not posted, and it does not fail the policy gate.
It **still counts against auto-approval**: a rule decides what the bot says,
never that a human may be skipped.

**Where rules are read from.** The bot reads the file from the **base
branch** (`git show <base>:.vizzini/suppressions.yaml`), so a rule takes
effect once it is merged. A rule added in a pull request does nothing for
that pull request, so an author cannot silence findings on their own change.

**From a dismissal to a rule.** When someone resolves one of the bot's
threads as *won't fix* or *by design*, the bot replies once with a rule
ready to commit. It covers that file and category, with the finding's
title offered as a narrower, commented-out option. Committing it is a
person's decision.

`pr-review --suppressions FILE` applies the same rules outside the bot.

## Thread memory

The bot keeps no state of its own; what it said lives in the PR's threads.
On every run it reads its own threads back by their markers and maps each
earlier anchor to the current head through `git diff`:

- **Already posted:** a finding with the same fingerprint as a thread, or
  the same file and category on overlapping (mapped) lines, is not posted
  again. Models reword titles between runs, so the fingerprint alone would
  repeat findings.
- **Fixed:** a thread is resolved only when its finding is no longer
  reported **and** the code under it changed. A finding that merely goes
  unreported while its code stands still is model variance, so its thread
  stays open and is listed as `kept_open`.
- **Dismissed:** a thread a person resolved stays resolved; the same finding
  is not posted again while its code is unchanged. If a fixed finding comes
  back after the code changed, that is a regression, and it gets a new
  thread.
- **Ownership:** mrman resolves only threads the token's own identity
  started, so a person's thread can never be closed by the bot.

Resolution and incremental review need the earlier commits in the
checkout: fetch full history in CI (`fetch-depth: 0`, or
`git fetch --unshallow`). Without them, earlier anchors cannot be mapped,
nothing is resolved, and every run is a full review.

## The policy is the grant

Interactively, mrman grants submit only to a person at a terminal
(`mrman pr URL --auto`), so an agent cannot authorize itself. The unattended
counterpart is a policy file that a person reviewed and committed. Start
from `deploy/ci/grant-policy/agent-submit.cue` (mrman's pack), commit it as
`.vizzini/grant-policy/agent-submit.cue`, and set its values:

```cue
_allowedEvents:     ["comment", "resolve", "reply"]  // never approve / request-changes unattended by default
_allowedIdentities: ["github-actions[bot]"]          // the identity your CI posts as
_maxComments:       50                                // a runaway review fails instead of flooding the PR
```

**The policy comes from the base branch.** A relative `--grant-policy` is
read from the base revision, not from the pull request's checkout, so a pull
request cannot loosen the policy it is judged by (say, to let itself
approve). A policy change takes effect once it is reviewed and merged. An
absolute path is the CI operator's own, for example baked into an image, and
is used as it is.

Allowing `approve` is a deliberate, recorded decision.

## Running in CI

`deploy/ci/` holds ready-made templates:

| Platform | Template | Posts as |
|---|---|---|
| GitHub Actions | `deploy/ci/github/vizzini-bot.yml` | `github-actions[bot]` |
| Azure Pipelines | `deploy/ci/azure-pipelines/vizzini-bot.yml` (build validation) | `<Project> Build Service (<org>)` |
| GitLab CI | `deploy/ci/gitlab/vizzini-bot.gitlab-ci.yml` | a project access token's bot user |

Each template:
- **Checks out** the pull request with full history.
- **Installs pinned tools** with `deploy/ci/install.sh`, verifying each:
  - `vizzini-bot`, built from a pinned Vizzini commit;
  - garmr at a pinned commit;
  - the mrman release, checked against its `SHA256SUMS`;
  - the tokenizer encodings, checked against pinned SHA-256 sums.
- **Runs the bot.**

Resolving threads and answering `/vizzini` commands need mrman driver
support that only Azure DevOps has so far. On GitHub and GitLab the bot
reviews, posts and deduplicates, and reports the rest as unsupported.

**The first run is denied, by design.** The starter policy lists no
identities. The first pull request's job therefore fails with exit 3, and
its log shows the identity your CI posts as:

```text
mrman: authorization: {"decision":"deny", ... "identity":"github-actions[bot]", ...}
```

Add that identity to the policy on your default branch in a reviewed change.
From then on, the bot posts. `deploy/ci/README.md` has the per-platform
setup, options and security notes.

## Try it without a forge

`--dry-run` reviews and prints the comments it would post as JSON lines. It
needs no `--pr-url` and no policy, and it never runs mrman.

```sh
vizzini-bot --base origin/main --dry-run
```

## Proven end to end

The project's UAT harness (vizzini-uat) runs `make uat-bot`
against a real Azure DevOps PR, with no person involved:

- **Deny leg:** the committed default policy denies the PAT's identity, and
  nothing is posted.
- **Allow leg:** a policy listing the identity mrman verified with the forge
  posts one marked thread per finding.

It also runs the bot again at the same head: incrementally it must make no
review, and with `--full` it must duplicate nothing. Then it pushes a fix,
which must be reviewed incrementally (that file only). The fixed defect's
thread must be resolved, and every thread on other files carried.

It then writes a `/vizzini explain …` and a malformed command in one of the
bot's threads. The explain must get a real answer, the malformed one usage,
and a further run must answer neither again.

Finally, a person dismisses one of the bot's threads. The bot must suggest a
rule; merged into the base branch as suggested, that rule must suppress the
finding on the next full review.

The GitHub Actions and Azure Pipelines templates run for real in the same
harness (`make uat-ci-github`, `make uat-ci-ado`): throwaway pull requests,
with stubllm in the job as the model.
- The first run is denied and names the identity.
- With the identity allowed on the base branch, the re-run posts its review
  comments.
- A second push is reviewed incrementally, with no duplicates.
- On Azure DevOps, a `/vizzini explain` command is answered as well.

Design: ADR-0016 to ADR-0021 (ISSUE-024).
