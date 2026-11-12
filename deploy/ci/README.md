# vizzini-bot in CI

These templates run [vizzini-bot](../../docs/src/content/docs/docs/guides/bot.md) on every pull request:

| Platform | Template | Posts as | Resolves threads, answers `/vizzini` |
|---|---|---|---|
| GitHub Actions | [`github/vizzini-bot.yml`](github/vizzini-bot.yml) | `github-actions[bot]` (`GITHUB_TOKEN`) | answers commands: yes. Resolves: with the `contents: write` opt-in (see Setup) |
| Azure Pipelines | [`azure-pipelines/vizzini-bot.yml`](azure-pipelines/vizzini-bot.yml) | `<Project> Build Service (<org>)` (`System.AccessToken`) | yes |
| GitLab CI | [`gitlab/vizzini-bot.gitlab-ci.yml`](gitlab/vizzini-bot.gitlab-ci.yml) | a project access token's bot user | yes (mrman v0.1.6 on) |

Every template does the same thing:
1. **Check out** the pull request with full history.
2. **Install** pinned tools with [`install.sh`](install.sh):
   - `vizzini-bot`, built from a pinned Vizzini commit;
   - garmr at a pinned commit;
   - the mrman release, checked against its `SHA256SUMS`;
   - the tokenizer encodings, checked against pinned SHA-256 sums.
3. **Run `vizzini-bot`** against the pull request.

With mrman v0.1.6 on, every platform resolves the bot's fixed threads and answers `/vizzini` commands (the templates pin v0.1.8). With an older mrman, GitHub and GitLab report those two as unsupported, and the bot still reviews, posts and deduplicates.

## Setup

1. **Commit a grant policy to your default branch.** Copy [`grant-policy/agent-submit.cue`](grant-policy/agent-submit.cue) to `.vizzini/grant-policy/agent-submit.cue`.
   - It allows `comment`, `resolve` and `reply`, never `approve` or `request-changes`, at most 50 comments per post, and only for the identities it lists.
   - It ships with **no** identities, so nothing is posted until you add yours (step 4).
   - The bot reads the policy from the **base branch**, so only a reviewed, merged policy change takes effect. A pull request cannot loosen the policy it is judged by.
2. **Add the template** for your platform and set its variables: the LiteLLM URL, the model, and a LiteLLM virtual key for the bot. Platform specifics:
   - **GitHub Actions:** to let the bot resolve its fixed threads, change the workflow's `contents: read` to `contents: write`. GitHub allows `GITHUB_TOKEN` to resolve review threads only with it, and it also lets the token push. Without it, the bot reports resolving as refused and goes on.
   - **Azure Pipelines:** grant the build service *Contribute to pull requests*.
   - **GitLab:** create a project access token with role Reporter or higher and scope `api`, and save it as the masked CI/CD variable `VIZZINI_GITLAB_TOKEN`. On a self-managed instance, the job lists the instance in mrman's config itself; with a private CA, add `ca_file` to that entry.
3. **Open a pull request.**
   - The bot reviews it and stops with exit code 3: *the grant policy denied posting; nothing was posted*.
   - The job log shows the denied decision, including the identity your CI posts as:

     ```text
     mrman: authorization: {"decision":"deny", ... "input":{"event":"comment","identity":"github-actions[bot]", ...}}
     ```
4. **Allow that identity.** Add it to `_allowedIdentities` in the policy, in a reviewed change on the default branch. From then on, the bot posts.

## Optional

- **Suppression rules:** commit them to `.vizzini/suppressions.yaml` on the default branch; the bot reads them from there.
- **Knowledge base:** add `--knowledge .okfw` (an OKF wiki) to ground findings in your design decisions.
- **Linter reports:** add `--lint-json FILE` (SARIF or golangci-lint JSON) so the bot explains your linters' diagnostics on changed lines.
- **Command limits:** `--command-authors` limits who may run `/vizzini` commands, and `--max-commands` caps how many are answered per run.

## Verification

- **GitHub Actions template:** runs live in vizzini-lab (`make uat-ci-github`). It plays out the setup above on a throwaway pull request: the denied first run, the identity allowed, comments posted, then an incremental second push with no duplicates. stubllm in the job stands in for the model.
- **Azure Pipelines template:** runs live in vizzini-lab (`make uat-ci-ado`), as PR build validation on a throwaway self-hosted agent.
  - It plays the same setup: the denied first build, the identity allowed, and threads posted. It also grants the build service *Contribute to pull requests*; without that grant, ADO refuses the post with `TF401027 PullRequestContribute`.
  - It then answers a `/vizzini explain` command on an incremental build.
- **GitLab CI template:** runs live in vizzini-lab (`make uat-ci-gitlab`), on a throwaway self-managed GitLab CE and gitlab-runner (docker executor) in podman.
  - It plays the same setup with a Reporter project access token: the denied first job naming the token's bot user, the identity allowed, and discussions posted as that bot user.
  - It then runs an incremental job with a `/vizzini explain` command, which goes unanswered: mrman cannot reply on GitLab yet, and the bot reports that and carries on.
  - gitlab.com itself is not run; there the job skips the mrman host entry and mrman reads `GITLAB_TOKEN`.
- **Lint:** `make ci-check` shellchecks `install.sh` and the GitLab job's script, runs actionlint on the GitHub workflow, and parses the Azure and GitLab YAML.

## Security notes

- **Forks:** the GitHub workflow runs only for pull requests from branches of the repository itself. Pull requests from forks get no secrets on `pull_request`. Never switch the workflow to `pull_request_target`, which would run with secrets on the fork's code.
- **Exposure:** the bot never executes code from the pull request; it reads the diff and the checkout. Its tokens can comment on pull requests in the repository, and its LiteLLM key spends against your proxy. Give it a virtual key with a budget.
- **Pins:** `VIZZINI_REF`, `MRMAN_VERSION` and `GARMR_REF` are pinned in the template. Change them in a reviewed commit.
