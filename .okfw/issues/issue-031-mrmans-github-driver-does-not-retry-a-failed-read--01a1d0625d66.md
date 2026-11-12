---
id: "01a1d062-5d66-7c0b-b9ce-c78616b9419f"
title: "ISSUE-031: mrman's GitHub driver does not retry a failed read"
type: "issue"
kind: "issue"
tags:
  - "issue"
  - "mrman"
  - "github"
  - "reliability"
  - "open"
created: "2026-11-12T09:31:09-06:00"
timestamp: "2026-11-12T09:31:09-06:00"
description: "One 503 from GitHub's API on a read failed a whole mrman command (and a vizzini-bot CI run); mrman's GitLab driver retries reads, its GitHub driver did not."
---

# ISSUE-031: mrman's GitHub driver does not retry a failed read

**Status:** open (fixed on mrman branch `fix/github-read-retry`, not yet merged) · **Raised:** 2026-11-12 · **Related:** [ISSUE-030](/issues/issue-030-mrman-reports-a-failed-identity-lookup-as-a-policy-denial--01a1d033165c.md)

## Problem
mrman's GitHub driver sent every request once. A transient 5xx from GitHub's API failed the whole command. vizzini-bot retries only mrman failures that say "network error", so a 503 ended the CI run.

**Seen live:** vizzini-lab's GitHub template test, run with the template's default permissions on mrman v0.1.8, failed twice: runs 20261112T150400Z and 20261112T152006Z. Inside the job, the first read, `GET /repos/infrashift/vizzini-lab/pulls/N`, got `HTTP 503`. The third attempt passed 15/15. githubstatus.com reported no incident. mrman's GitLab driver already retried reads (`gitlabf/retry.go`); the GitHub driver did not.

## Fix
Both GitHub clients, REST and GraphQL, go through a transport that resends only what cannot act twice. This is the GitLab driver's rule, with up to three attempts and backoff:
- **GET and HEAD:** on a 5xx other than 501, or on a transport error.
- **Any method:** on an error raised before a connection existed (dial, DNS).
- **Never:** a write (POST: a REST create, a GraphQL mutation) that got a 5xx or lost its connection mid-way. GitHub may already have applied it.

**Tests:** `internal/forge/githubf/retry_test.go`:
- reads retried on 502, 503 and 504;
- giving up after three attempts;
- no resend of a POST after a 5xx, and no retry of a 4xx or a 501;
- transport errors by method;
- a cancelled context stops the retries;
- the driver's own PR read recovers from a 503.

## Acceptance criteria
- [x] The fix and the tests; mrman `make check` passes (87.5% coverage).
- [x] The forge capabilities reference says which requests are resent.
- [ ] Merged and released in mrman, pinned in the CI templates and vizzini-lab.
- [ ] The vizzini-lab GitHub suites pass on the release.
