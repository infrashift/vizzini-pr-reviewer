---
x-okfw-id: "01a205a9-72f2-7581-92f0-b003bb7f0e54"
title: "ADR-0026: install.sh keeps verified tools in a cache under their pins"
type: "ADR"
tags:
  - "adr"
  - "ci"
  - "security"
x-okfw-created: "2026-11-22T17:48:40-06:00"
x-okfw-timestamp: "2026-11-22T17:48:40-06:00"
description: "On a self-hosted agent, install.sh keeps each verified binary in VIZZINI_TOOLS_CACHE under its pin, in entries that never change, and uses one only while it matches its recorded SHA-256."
---

# ADR-0026: install.sh keeps verified tools in a cache under their pins

**Status:** Accepted
**Date:** 2026-11-22
**Related:** [ISSUE-036](/issues/issue-036-install-sh-downloads-every-tool-on-every-job-also-on-a-self-hosted-agent--01a205a97301.md), [ADR-0025](/decisions/adr-0025-release-archives-for-linux-amd64-and-arm64--01a1ffcd31a4.md) (release archives), [ISSUE-035](/issues/issue-035-install-sh-does-not-retry-a-reset-download--01a2007408e3.md) (download retries)

## Context
The CI templates run `deploy/ci/install.sh` in every job. On a hosted runner, each job starts on a new machine, so it must download the tools. A persistent self-hosted agent downloads and verifies the same pinned tools again on every job, about 45 s each time, and a network failure can stop the job. One agent can run pipelines with different pins, and several agents on one machine can run jobs at the same time.

## Decision
- **Opt-in:** `VIZZINI_TOOLS_CACHE` names a cache directory. Without it, `install.sh` works as before. The templates set it to a directory that stays between jobs on a self-hosted agent: `$RUNNER_TOOL_CACHE/vizzini`, `$AGENT_TOOLSDIRECTORY/vizzini` or `$CI_BUILDS_DIR/.vizzini-cache`.
- **Entries under their pins:** `<cache>/<tool>/<key>/`, where the key is the release archive name (`vizzini_<version>_<os>_<arch>`, `mrman_<version>_<os>_<arch>`, `garmr_<version>_<os>_<arch>`; garmr comes from a release since [ADR-0027]({ADR27})). The job still gets its own copy in `TOOLS_DIR/bin`, so a job never runs a binary that another job can change.
- **Entries never change:** a new entry is written to a temporary directory in the cache and renamed into place. If another job added it first, the script keeps that one. Damaged entries are replaced.
- **Checks on every job:** an entry is used only if its binary matches the SHA-256 recorded when it was cached. The `vizzini-bot` commit check runs on a cached binary too. `tiktoken-cache.sh` checks the cached encodings against their pinned sums.
- **No version probe:** the script does not trust a binary on `PATH` because its `--version` matches. The pin selects the entry, and the recorded hash checks it.

## Rejected
- **A shared `bin/` that is updated in place:** a job with other pins would replace binaries that a running job uses.
- **Keep the release archives and check them against a fresh `SHA256SUMS` on every job:** it still needs the network, and it does not protect against a job that can write the cache, because that job can also change the agent itself.
- **The platforms' cache tasks (`Cache@2`, `actions/cache`):** they differ on each platform, they upload and download the cache over the network, and a hosted runner gains nothing from them.

## Consequences
- A warm self-hosted job installs the tools in under 1 s, without the network.
- The cache is only as trustworthy as the agent: any job that runs as the agent's user can write it, and the recorded SHA-256 catches damage, not a change. On an agent that also runs untrusted code, remove `VIZZINI_TOOLS_CACHE` from the template.
- The script never removes old entries. Operators delete the directory to clear it.
