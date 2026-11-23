---
x-okfw-id: "01a20944-7b95-758f-9d21-605eccbbeda4"
title: "ADR-0029: On Azure DevOps Server the collection path takes the organization's place"
type: "ADR"
tags:
  - "adr"
  - "azure-devops"
  - "forge"
x-okfw-created: "2026-11-23T10:36:52-06:00"
x-okfw-timestamp: "2026-11-23T10:36:52-06:00"
description: "Azure DevOps Server repositories are keyed by host, collection path (virtual directory included), project and repository, anchored on _git, so Services and Server share one key shape and Services keys do not change."
---

# ADR-0029: On Azure DevOps Server the collection path takes the organization's place

# ADR-0029: On Azure DevOps Server the collection path takes the organization's place

**Status:** Accepted
**Date:** 2026-11-23
**Source:** request to support Azure DevOps Server next to Azure DevOps Services
**Related:** [ISSUE-038](/issues/issue-038-support-azure-devops-server-repository-urls--01a20944a682.md), [ISSUE-025](/issues/issue-025-usage-and-cost-attribution-per-request-review-run-and-pr-lifecycle--01a1cba3cacc.md) (pr_key format), [ADR-0021](/decisions/adr-0021-ci-templates-install-pinned-verified-tools-the-grant-policy-is-read-from-the-base-branch--01a1ccc9f271.md) (self-managed GitLab precedent), [ISSUE-026](/issues/issue-026-webhook-receiver-answers-vizzini-commands-immediately-experimental--01a1cf8f51ab.md) (webhook receiver)

## Context
Azure DevOps Services repository URLs have the shape `https://dev.azure.com/{org}/{project}/_git/{repo}`. Azure DevOps Server URLs have the shape `https://tfs.example.com/tfs/DefaultCollection/{project}/_git/{repo}`. The collection has the role of the organization. A virtual directory (`tfs/`) can come before it, and some servers have none.

vizzini-webhook accepted only `dev.azure.com` and exactly `{org}/{project}/_git/{repo}`. vizzini-bot resolved its status thread only on Services hosts. mrman counted path segments in the same way. The CI template already formed the key `ado:${rest%%/_git/*}/${rest##*/_git/}#N`, which works for any host.

## Decision
- **No collection concept.** The collection path, virtual directory included, takes the organization's place: `tfs/DefaultCollection`.
- **Anchor on `_git`.** Everything before `{project}/_git/{repo}` is the collection path. At least one segment must come before the project.
- **One key shape:** `HOST/COLLECTION-PATH/PROJECT/REPO`. On Services, the key stays `dev.azure.com/ORG/PROJECT/REPO`, so cost history and existing keys do not change.
- **The API base is `https://HOST/COLLECTION-PATH`.** mrman builds it from the owner, and the owner now holds the collection path. `api_base` stays the override for plain HTTP or a non-default port. In CI it comes from `System.CollectionUri`.
- **Minimum version: Azure DevOps Server 2022.1.** mrman uses REST api-version 7.1.
- **The webhook accepts any host.** The `--repo` allowlist decides which repositories the receiver serves.
- **The bot recognizes a Server pull request by its path**, `/…/{project}/_git/{repo}/pullrequest/{id}`, so it resolves its status thread there too.

## Consequences
- A Server host needs a `[[forge.hosts]]` entry in mrman's config (`forge = "azuredevops"` and a token). The Azure Pipelines template writes the entry itself, as the GitLab template does for a self-managed instance.
- The webhook clones over HTTPS only.
- Legacy `ORG.visualstudio.com/PROJECT/_git/REPO` URLs without `DefaultCollection` have no collection path, and the webhook refuses them as before. With `DefaultCollection` they now work.
- The project-named repository shorthand `{collection}/_git/{repo}` stays unsupported.

## Alternatives considered
- **A separate collection field.** Rejected: one more concept in every key and URL for no gain.
- **Collection name only, with the virtual directory in `api_base`.** Rejected: every Server host would need configuration to parse its own URLs, PR and clone URLs would lose the prefix, and webhook keys would no longer match CI keys.
- **An api-version fallback for Server 2019 and 2020.** Deferred: the SDK pins its own versions.
