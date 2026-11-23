---
x-okfw-id: "01a20944-a682-7136-9466-db1778930713"
title: "ISSUE-038: Support Azure DevOps Server repository URLs"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "closed"
  - "phase-4"
  - "azure-devops"
x-okfw-created: "2026-11-23T10:37:03-06:00"
x-okfw-timestamp: "2026-11-23T10:37:03-06:00"
description: "The webhook, the bot, the CI template and mrman accepted only Azure DevOps Services URLs; Azure DevOps Server URLs put a collection path, often behind a tfs/ virtual directory, in the organization's place."
---

# ISSUE-038: Support Azure DevOps Server repository URLs

# ISSUE-038: Support Azure DevOps Server repository URLs

**Status:** closed · **Phase:** 4 (Milestone 4.8) · **Raised:** 2026-11-23 · **Related:** [ADR-0029](/decisions/adr-0029-on-azure-devops-server-the-collection-path-takes-the-organizations-place--01a209447b95.md), [ISSUE-026](/issues/issue-026-webhook-receiver-answers-vizzini-commands-immediately-experimental--01a1cf8f51ab.md)

## Description
Azure DevOps Server URLs have the shape `https://tfs.example.com/tfs/DefaultCollection/{project}/_git/{repo}`. vizzini-webhook accepted only `dev.azure.com/{org}/{project}/_git/{repo}`. vizzini-bot resolved its status thread only on Services hosts. The cost-attribution example built a wrong key on Server. mrman counted path segments, so the extra `tfs/` segment failed its remote, PR URL and coordinate parsers.

## Acceptance Criteria
- [x] mrman: remotes, PR URLs and host-qualified coordinates with a collection path resolve, with the collection path as the owner. The org URL is the collection URL (mrman PR #24, released in v0.1.11).
- [x] vizzini-webhook: `normalizeRepo` accepts any host and anchors on `_git`. Keys and clone URLs keep the collection path. Services keys do not change.
- [x] vizzini-bot: `azureDevOps` recognizes a Server pull request URL by its path.
- [x] Azure Pipelines template: on a Server host, the job writes a `[[forge.hosts]]` entry from `System.CollectionUri`.
- [x] `ado-subscription.sh` takes `ADO_COLLECTION_URL`.
- [x] The cost-attribution example uses the template's `_git`-anchored key.
- [x] Docs: webhook README and guide, CI README, env example.
- [x] Tests: Server URLs with and without a virtual directory, keys, clone and web URLs, rejects.

## Notes
- Minimum version: Azure DevOps Server 2022.1 (REST api-version 7.1).
- A plain HTTP or non-default-port server needs `api_base` in mrman's config. The webhook clones over HTTPS only.
- mrman v0.1.11 (2026-11-23, mrman PR #24) ships the Server parsing, and Vizzini v0.1.4 ships the webhook and bot changes. The templates pin both.

## Resolution
Done as [ADR-0029](/decisions/adr-0029-on-azure-devops-server-the-collection-path-takes-the-organizations-place--01a209447b95.md) describes. The collection path takes the organization's place, and every parser anchors on `_git`. Follow-ups: an api-version fallback for Server before 2022.1, webhook clones over HTTP, and the vizzini-lab UAT scripts (they hard-code dev.azure.com).
