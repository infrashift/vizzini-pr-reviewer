#!/usr/bin/env bash
# ado-subscription.sh: manage the Azure DevOps service hook that feeds
# vizzini-webhook (EXPERIMENTAL). See deploy/webhook/README.md.
#
#   ado-subscription.sh create    one "Pull request commented on" Web Hooks
#                                 subscription for ADO_REPO, to WEBHOOK_URL
#   ado-subscription.sh list      this project's vizzini-webhook subscriptions
#   ado-subscription.sh delete ID remove one subscription
#
# Configuration (environment; nothing secret on a command line):
#   ADO_ORG, ADO_PROJECT, ADO_REPO   the repository the hook watches
#   ADO_BRANCH                       optional: only PRs into this branch
#   WEBHOOK_URL                      https://HOST/hooks/azure-devops
#   WEBHOOK_USER                     basic-auth user (default: vizzini)
#   WEBHOOK_SECRET_FILE              file with the basic-auth password
#   ADO_ADMIN_PAT_FILE               file with a PAT of a user allowed to
#                                    manage service hooks (Project
#                                    Administrator); used only here
set -euo pipefail

die() { printf 'ado-subscription: %s\n' "$*" >&2; exit 1; }
need() { [[ -n ${!1:-} ]] || die "set $1"; }
need ADO_ORG
need ADO_PROJECT
need ADO_ADMIN_PAT_FILE
[[ -r $ADO_ADMIN_PAT_FILE ]] || die "cannot read ADO_ADMIN_PAT_FILE"
hooks="https://dev.azure.com/$ADO_ORG/_apis/hooks"

# api METHOD URL [JSON]: the body; fails on a non-2xx status. The PAT goes
# to curl in a config on stdin; a request body (it may hold the webhook
# password) goes through a private temporary file.
umask 077
api() {
	local out code data=()
	out="$(mktemp)"
	if [[ -n ${3:-} ]]; then
		printf '%s' "$3" >"$out.body"
		data=(--data-binary "@$out.body")
	fi
	code="$(printf 'header = "Authorization: Basic %s"\n' "$(printf ':%s' "$(tr -d '\r\n' <"$ADO_ADMIN_PAT_FILE")" | base64 -w0)" |
		curl -sS -K - -o "$out" -w '%{http_code}' -X "$1" -H 'Content-Type: application/json' "${data[@]}" "$2")" || true
	rm -f "$out.body"
	if [[ $code != 2* ]]; then
		echo "ado-subscription: $1 $2 -> HTTP $code: $(head -c 400 "$out")" >&2
		rm -f "$out"
		return 1
	fi
	cat "$out"
	rm -f "$out"
}
py() { python3 -I -c "$@"; }

case "${1:-}" in
create)
	need ADO_REPO
	need WEBHOOK_URL
	need WEBHOOK_SECRET_FILE
	[[ $WEBHOOK_URL == https://* ]] || die "WEBHOOK_URL must be https:// (ADO sends basic auth only over HTTPS)"
	[[ -r $WEBHOOK_SECRET_FILE ]] || die "cannot read WEBHOOK_SECRET_FILE"
	project_id="$(api GET "https://dev.azure.com/$ADO_ORG/_apis/projects/$ADO_PROJECT?api-version=7.1" | py 'import json,sys; print(json.load(sys.stdin)["id"])')"
	repo_id="$(api GET "https://dev.azure.com/$ADO_ORG/$ADO_PROJECT/_apis/git/repositories/$ADO_REPO?api-version=7.1" | py 'import json,sys; print(json.load(sys.stdin)["id"])')"
	# The body carries the webhook password, so it is built from the file
	# and handed to curl on stdin, never on a command line.
	body="$(WEBHOOK_USER="${WEBHOOK_USER:-vizzini}" py '
import json, os, sys
project, repo, branch, url, secret_file = sys.argv[1:]
secret = open(secret_file).read().strip()
pub = {"projectId": project, "repository": repo}
if branch:
    pub["branch"] = branch
print(json.dumps({
    "publisherId": "tfs",
    "eventType": "ms.vss-code.git-pullrequest-comment-event",
    "resourceVersion": "2.0",
    "consumerId": "webHooks",
    "consumerActionId": "httpRequest",
    "publisherInputs": pub,
    "consumerInputs": {
        "url": url,
        "basicAuthUsername": os.environ["WEBHOOK_USER"],
        "basicAuthPassword": secret,
        "resourceDetailsToSend": "",
        "messagesToSend": "none",
        "detailedMessagesToSend": "none",
        "acceptUntrustedCerts": "false",
    },
}))' "$project_id" "$repo_id" "${ADO_BRANCH:-}" "$WEBHOOK_URL" "$WEBHOOK_SECRET_FILE")"
	api POST "$hooks/subscriptions?api-version=7.1" "$body" |
		py 'import json,sys; d=json.load(sys.stdin); print("created subscription", d["id"], "status", d["status"])'
	;;
list)
	project_id="$(api GET "https://dev.azure.com/$ADO_ORG/_apis/projects/$ADO_PROJECT?api-version=7.1" | py 'import json,sys; print(json.load(sys.stdin)["id"])')"
	api GET "$hooks/subscriptions?publisherId=tfs&eventType=ms.vss-code.git-pullrequest-comment-event&consumerId=webHooks&api-version=7.1" |
		py '
import json, sys
project = sys.argv[1]
for s in json.load(sys.stdin)["value"]:
    if s["publisherInputs"].get("projectId") == project:
        print(s["id"], s["status"], s["consumerInputs"].get("url"), "repository=" + s["publisherInputs"].get("repository", "*"),
              "branch=" + s["publisherInputs"].get("branch", "*"))' "$project_id"
	;;
delete)
	[[ ${2:-} =~ ^[0-9a-f-]{36}$ ]] || die "usage: $0 delete SUBSCRIPTION_ID"
	api DELETE "$hooks/subscriptions/$2?api-version=7.1" >/dev/null && echo "deleted subscription $2"
	;;
*)
	sed -n '2,20p' "$0" >&2
	exit 2
	;;
esac
