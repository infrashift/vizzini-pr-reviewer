# vizzini-webhook: answer `/vizzini` commands immediately

> **EXPERIMENTAL and OPTIONAL.** Without this receiver, nothing changes: the CI templates in [`deploy/ci/`](../ci/) review every push, and a `/vizzini` command is answered by the next pipeline run. Deploy the receiver only if you want commands answered as they are written. It supports **Azure DevOps Services only** (`dev.azure.com`), because mrman cannot reply in GitHub or GitLab threads yet. Flags, files and behavior can change between releases.

## What it does
```
PR comment "/vizzini explain …"
   │  Azure DevOps service hook (HTTPS, basic auth)
   ▼
reverse proxy (TLS) ──► vizzini-webhook ──► vizzini-bot --commands-only ──► mrman ──► reply in the thread
                          │ authenticate, filter, queue        │ grant policy from the base branch (garmr)
                          ▼                                    ▼
                    cached clone of the repository       LiteLLM (one model call per command)
```

1. **Delivery.** Azure DevOps posts every PR comment in the repository to `https://HOST/hooks/azure-devops`, with HTTP basic auth.
2. **Checks.** The receiver checks the credentials in constant time. It acts only on a text comment with a `/vizzini` line, on an **active** PR, in an **allowlisted** repository, that the bot did not write. It ignores every other comment with `200 {"accepted":false,"reason":…}`.
3. **Queue.** It answers `202` at once and queues one job per PR. More comments on a PR whose job waits join that job. A comment on a PR whose job runs makes the job run once more after it ends.
4. **Job.** The receiver fetches the target branch and `refs/pull/<id>/merge` into a cached clone and checks out the PR's head. It then runs `vizzini-bot --commands-only`. That run answers the pending commands, up to `--max-commands`, and does nothing else: **no review, no resolved threads, no suggestions.** Reviews stay with CI.
5. **Authorization.** It is unchanged. mrman posts each reply only if the grant policy on the PR's **base branch** allows event `reply` for the receiver's identity. Each command is still answered once.

## Before you start
| You need | Why |
|---|---|
| A Linux host with systemd, reachable from the internet on HTTPS | Azure DevOps cannot deliver to localhost, private addresses or plain HTTP. |
| A DNS name and a TLS certificate for it (the Caddy example gets one automatically) | ADO sends basic auth only over HTTPS. |
| Outbound access from the host to `dev.azure.com`, your LiteLLM proxy, `github.com` and `proxy.golang.org` (install only) | Fetching, replying, model calls, installing. |
| Go (any version from Vizzini's `go.mod` on), git, curl, python3 | Installing and running. |
| A **dedicated Azure DevOps account** for the bot, with access to the repository | Replies appear under this account. Do not use a person's account. |
| A **PAT for that account**, scope *Code (Read & Write)* | Fetching the repository and replying in threads. |
| A LiteLLM **virtual key with a budget** | Each answered command is one model call. |
| A person with **Project Administrator** rights, for one step | Creating the service hook subscription. |
| The repository already uses the bot's grant policy (`.vizzini/grant-policy/` on the default branch) | See [`deploy/ci/README.md`](../ci/README.md). |

## Runbook

### 1. Install the tools
On the host, as root:
```sh
useradd --system --home-dir /var/lib/vizzini-webhook --shell /usr/sbin/nologin vizzini-webhook
git clone https://github.com/infrashift/vizzini-pr-reviewer.git /opt/vizzini-src
cd /opt/vizzini-src && git checkout <PINNED_COMMIT>     # the Vizzini commit you run in CI, or newer
deploy/ci/install.sh /opt/vizzini                    # vizzini-bot, mrman, garmr, tokenizer files; all verified
CGO_ENABLED=0 go build -trimpath -o /opt/vizzini/bin/vizzini-webhook ./cmd/vizzini-webhook
/opt/vizzini/bin/vizzini-webhook --help              # prints the flags; exits 0
```
**Expected:** `install: done` from `install.sh`, and `/opt/vizzini/bin/` holds `vizzini-bot`, `vizzini-webhook`, `mrman` and `garmr`.

### 2. Write the configuration and the secrets
```sh
install -d -m 0755 /etc/vizzini-webhook
install -m 0644 deploy/webhook/vizzini-webhook.env.example /etc/vizzini-webhook/vizzini-webhook.env
install -m 0600 deploy/webhook/secrets.env.example        /etc/vizzini-webhook/secrets.env
head -c 32 /dev/urandom | base64 | tr -d '\n' > /etc/vizzini-webhook/webhook.secret
chmod 0600 /etc/vizzini-webhook/webhook.secret
```
- **Repository:** in `vizzini-webhook.env`, set `WEBHOOK_REPO=dev.azure.com/ORG/PROJECT/REPO`, plus `VIZZINI_LITELLM_URL` and `VIZZINI_MODEL`. For more than one repository, add more `--repo` flags to `ExecStart` in the unit.
- **Secrets:** in `secrets.env`, set `AZURE_DEVOPS_EXT_PAT` (the bot account's PAT) and `VIZZINI_API_KEY`. Use an editor, never `echo` on a shared shell.
- **Password:** `webhook.secret` is the basic-auth password. It must be at least 16 characters. Keep it: step 6 needs it.

### 3. Start the service
```sh
install -m 0644 deploy/webhook/vizzini-webhook.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now vizzini-webhook
systemctl status vizzini-webhook --no-pager           # active (running)
curl -fsS http://127.0.0.1:8484/healthz                # ok
journalctl -u vizzini-webhook -n 5 --no-pager         # "vizzini-webhook is EXPERIMENTAL", then "listening"
```

### 4. Expose it over HTTPS
Put a TLS reverse proxy in front of `127.0.0.1:8484` that forwards only `/hooks/azure-devops` and `/healthz`. [`Caddyfile.example`](Caddyfile.example) does this and manages the certificate. Alternatively, run the receiver with `--tls-cert` and `--tls-key`.

From a machine outside your network:
```sh
curl -fsS https://hooks.example.com/healthz                             # ok
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://hooks.example.com/hooks/azure-devops   # 401
```

### 5. Allow the bot account in the grant policy
The receiver posts as the bot account, and the policy on the **base branch** decides whether it may. In a reviewed PR to the default branch, add the account's identity to `_allowedIdentities` in `.vizzini/grant-policy/agent-submit.cue`. **Keep the CI identity there too.**
```cue
_allowedIdentities: ["<Project> Build Service (<org>)", "Vizzini Bot"]
```
The identity is the account's **display name**, as Azure DevOps reports it. If you are not sure, skip this step. The first command in step 7 is then denied, and the receiver's log names the exact value: `add "Vizzini Bot" to _allowedIdentities in the base branch's policy`.

### 6. Create the service hook subscription
Use the script, run by the Project Administrator. Their PAT is used only for this step.
```sh
export ADO_ORG=ORG ADO_PROJECT=PROJECT ADO_REPO=REPO
export WEBHOOK_URL=https://hooks.example.com/hooks/azure-devops
export WEBHOOK_SECRET_FILE=/path/to/a/copy/of/webhook.secret     # the password from step 2
export ADO_ADMIN_PAT_FILE=/path/to/admin.pat                       # 0600
deploy/webhook/ado-subscription.sh create     # created subscription <id> status enabled
deploy/webhook/ado-subscription.sh list
```
Or do it in the web UI: *Project settings → Service hooks → + → Web Hooks*.
- **Trigger:** *Pull request commented on*. Set *Repository* to the repository and *Target branch* to "[Any]" or your default branch.
- **Action:**
  - URL: `https://hooks.example.com/hooks/azure-devops`
  - Basic authentication: user `vizzini`, and the password from step 2
  - *Resource details to send*: **All**
  - *Messages to send* and *Detailed messages to send*: **None**
- Leave *Accept untrusted SSL certificates* off.

In the UI, *Test* sends Microsoft's sample event, whose repository is not yours. The receiver answers it with `400` (unsupported legacy host) or `200 … repository not allowlisted`, which proves the network path and the password. A `401` means the password differs.

### 7. Verify end to end
1. In an open PR of the repository, write a comment on a changed line: `/vizzini explain What does this do?`.
2. In the receiver's log (`journalctl -u vizzini-webhook -f`), expect three lines: `delivery accepted` with `"queue":"queued"`, then `job started`, then `job done` with `"answered":["<comment id>"]`.
3. In the PR, a reply from the bot account appears in the same thread, usually within a minute.
4. In *Project settings → Service hooks*, the subscription's history shows the delivery with response `202`.

### 8. Record it
Write down the host, the Vizzini commit, the subscription id and the date. Every upgrade and every secret rotation starts from that record.

## Operations
- **Logs:** run `journalctl -u vizzini-webhook`. The logs are JSON, one line per delivery and per job. They never contain the password or the tokens, and they shorten the requester's identity to three characters.
- **Upgrade:** check out the new commit in `/opt/vizzini-src`, run `install.sh` again, rebuild `vizzini-webhook`, then `systemctl restart vizzini-webhook`. A restart waits for running jobs; waiting deliveries are dropped, and the next comment or CI run answers them.
- **Rotate the webhook password:** write a new `webhook.secret`, then update the subscription's password (UI *Edit*, or `delete` and `create`), then restart the service. Deliveries between the two steps fail with `401`, and ADO retries them.
- **Rotate the bot's PAT:** update `secrets.env`, then restart.
- **Cost:** each answered command is one model call, attributed like a CI run (`--pr`, `--requester`). Bound it with `--max-commands`, `--command-authors` (add `--command-authors alice,bob` to `ExecStart`) and the LiteLLM key's budget.
- **Turn it off:** disable or delete the subscription (`ado-subscription.sh delete <id>`), then `systemctl disable --now vizzini-webhook`. Commands are then answered by the next CI run again.

## Troubleshooting
| Symptom | Cause | Fix |
|---|---|---|
| ADO history: `401` | The password or user differs between the subscription and `webhook.secret` | Set both to the same values. |
| ADO cannot deliver, or the subscription goes on probation | The URL is not public HTTPS, or the certificate is not trusted | Use a public name with a valid certificate; check `/healthz` from outside. |
| Log: `delivery ignored` with `repository not allowlisted` | The repository is not in `--repo` | Add `--repo dev.azure.com/ORG/PROJECT/REPO`. |
| Log: `delivery refused: bad payload … resource details` | The subscription sends minimal or no resource details | Set *Resource details to send* to **All**. |
| Log: `delivery refused: bad payload … only dev.azure.com` | The organization uses a legacy `*.visualstudio.com` URL | Not supported yet. |
| Log: `the grant policy denied the reply … add "<name>"` | The bot account is not in the base branch's policy | Step 5, with the name from the log. |
| Log: `job failed … git fetch` | The PAT cannot read the repository, or it expired | Check the bot account's access and the PAT's scope and expiry. |
| A command is answered twice | A CI run and a receiver job answered at the same moment | Rare; a known limit of the experiment. Delete one reply. |
| No reply and no log line | The comment has no `/vizzini` line of its own, or the subscription filters another repository or branch | Check the comment and `ado-subscription.sh list`. |

## Security
- **Transport and secrets:** HTTPS only, and basic auth compared in constant time. The secrets live in root-only files: the receiver reads the password as a systemd credential, and the tokens arrive in its environment. None is ever on a command line.
- **Least privilege:** the bot account needs *Code (Read & Write)* on the repository, nothing more. The service runs as an unprivileged user with a read-only system (`ProtectSystem=strict`) and a private state directory.
- **What a comment can trigger:** an answer, and nothing else. The receiver never reviews, resolves, approves or merges, and it never runs code from the pull request. The grant policy comes from the base branch, so a PR cannot widen what the bot may do.
- **Exposure:** anyone who can comment on the PR can spend model calls. Limit that with `--command-authors` and the key's budget.

## Related
- [`deploy/ci/README.md`](../ci/README.md): the CI templates. They stay the way PRs are reviewed.
- The docs site guide: *Webhook Receiver (Experimental)*.
- Vizzini ADR-0022 and ISSUE-026 in `.okfw/`: the design decision and what is open.
