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

stdout is one JSON object for the run:

```json
{"run_id":"…","head":"…","mode":"incremental","since":"…","reviewed_files":["src/a.cs"],
 "findings":2,"posted":1,"already_posted":["v1:…"],"resolved":["197"],"kept_open":[],
 "carried":["195"],"submit":{…},"resolve":{…}}
```

`mode` is `full`, `incremental` or `unchanged` (no review was needed).

| exit | meaning |
|---|---|
| 0 | done: posted, resolved, or there was nothing to do |
| 1 | an error (git, review, mrman) |
| 3 | the grant policy denied posting or resolving |

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

A run that posts nothing leaves no marker, so the next run reviews from the
last head that did post: more than needed, never less. On the UAT's C#
fixture, an incremental review of a one-file push cost about 1/7 of a full
review.

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
counterpart is a policy file that a person reviewed and committed. Copy
mrman's pack (`policies/garmr/mrman`) into your repository and set its
values:

```cue
_allowedEvents:     ["comment", "resolve"]  // never approve / request-changes unattended by default
_allowedIdentities: ["vizzini-bot"]         // the bot account's login (Azure DevOps: its display name)
_maxComments:       50                      // a runaway review fails instead of flooding the PR
```

Changes to that file go through code review like any other change.
Allowing `approve` is a deliberate, recorded decision.

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

Design: ADR-0016, ADR-0017 and ADR-0018. Next slices (ISSUE-024): in-thread
`/vizzini` commands, and suppression rules from dismissed findings.
