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

1. **Review.** `git diff base...head` in `--repo` (default `.`) is reviewed.
   The checkout supplies [repository context](../repository-context/), and
   you can add a [knowledge base](../knowledge/) and
   [linter reports](../lint/). Every request carries
   [attribution](../cost-attribution/): agent `vizzini-bot`, the head commit
   as the iteration, and the run ID.
2. **Draft.** `mrman pr URL --json` opens the PR headlessly, and
   `mrman review add` drafts one comment per finding. Each comment ends with
   a hidden marker, `<!-- vizzini:fp=v1:… run=… -->`. The fingerprint is
   built from the file, the category and the normalized title, not the
   lines, so a later run can recognize the same finding after code moves.
3. **Authorize and post.**
   `mrman review submit --event comment --grant-policy DIR` asks garmr
   whether this event, by this identity, may be posted.
   - The identity is what the forge reports for the token, not anything the
     bot claims.
   - On allow, mrman posts. On deny, or if the policy cannot be evaluated,
     nothing is posted.
   - Every decision goes to stderr and to mrman's audit log, with the policy
     digest and the run ID.

| exit | meaning |
|---|---|
| 0 | posted, or there was nothing to post |
| 1 | an error (git, review, mrman) |
| 3 | the grant policy denied posting; nothing was posted |

mrman calls that fail with a network error are retried, up to three
attempts. A submit that failed that way keeps its unposted comments for the
next attempt.

## The policy is the grant

Interactively, mrman grants submit only to a person at a terminal
(`mrman pr URL --auto`), so an agent cannot authorize itself. The unattended
counterpart is a policy file that a person reviewed and committed. Copy
mrman's pack (`policies/garmr/mrman`) into your repository and set its
values:

```cue
_allowedEvents:     ["comment"]       // never approve / request-changes unattended by default
_allowedIdentities: ["vizzini-bot"]   // the bot account's login (Azure DevOps: its display name)
_maxComments:       50                // a runaway review fails instead of flooding the PR
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

Design: ADR-0016. Next slices (ISSUE-024): thread memory (skip already-posted
findings, resolve fixed ones), incremental review, in-thread `/vizzini`
commands.
