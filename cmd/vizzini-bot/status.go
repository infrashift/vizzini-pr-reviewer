package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// The status comment (ADR-0028). A review that posts nothing (no finding,
// no resolution, no answer, no suggestion) would leave no head marker, and
// the next run would review again from the last head that did. Such a run
// records its head in one pull-request-level comment instead: the bot
// posts it once and then edits it (mrman review edit), so a pull request
// gets at most one status comment, however many quiet pushes it has.
//
// On Azure DevOps a pull-request-level comment is a thread, created active.
// A branch policy that requires every comment to be resolved would then
// block the pull request on the status comment, so the bot resolves that
// thread once it is posted. GitHub review bodies and GitLab general notes
// have no such state.

const (
	// statusMarker identifies the bot's status comment.
	statusMarker = "<!-- vizzini:status -->"
	// maxStatusHeads is how many reviewed heads the status comment keeps,
	// newest last. Older heads only matter after a force push, and the
	// findings' own markers still record theirs.
	maxStatusHeads = 20
)

// statusHeadRE finds one recorded head in a status comment.
var statusHeadRE = regexp.MustCompile(`<!-- vizzini:reviewed head=[0-9a-f]{7,64} run=\S+ -->`)

// remoteSummary is a pull-request-level comment, as `mrman review threads`
// lists it under summaries.
type remoteSummary struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
}

// recalled is what the bot reads back from the pull request.
type recalled struct {
	slug    string
	threads []remoteThread
	// summaries are the pull-request-level comments. listsSummaries is
	// false when mrman is too old to list them (before review edit): the
	// bot then cannot find its status comment, so it writes none.
	summaries      []remoteSummary
	listsSummaries bool
}

// statusComments are the summaries that carry the status marker.
func statusComments(summaries []remoteSummary) []remoteSummary {
	var out []remoteSummary
	for _, s := range summaries {
		if strings.Contains(s.Body, statusMarker) {
			out = append(out, s)
		}
	}
	return out
}

// statusHeads lists the heads the status comments record.
func statusHeads(summaries []remoteSummary) []string {
	status := statusComments(summaries)
	threads := make([]remoteThread, 0, len(status))
	for _, s := range status {
		threads = append(threads, remoteThread{Comments: []remoteComment{{Body: s.Body}}})
	}
	return reviewedHeads(threads)
}

// statusBody is the status comment after a quiet review of head: the heads
// earlier recorded in previous (if any), then head, at most maxStatusHeads.
// A head recorded before moves to the end, under this run.
func statusBody(previous, head, runID string) string {
	lines := slices.DeleteFunc(statusHeadRE.FindAllString(previous, -1), func(l string) bool {
		return strings.Contains(l, " head="+head+" ")
	})
	lines = append(lines, fmt.Sprintf("<!-- vizzini:reviewed head=%s run=%s -->", head, runID))
	if len(lines) > maxStatusHeads {
		lines = lines[len(lines)-maxStatusHeads:]
	}
	return fmt.Sprintf("**%s** reviewed this pull request through `%.12s` (run `%s`) and found nothing new to post.\n\n"+
		"_%s edits this comment after each review that posts nothing, so that the next review starts from the last reviewed commit._\n\n"+
		"%s\n%s", agentName, head, runID, agentName, statusMarker, strings.Join(lines, "\n"))
}

// recordedThisRun reports whether this run posted something that records
// its head: a finding, a resolution, an answer or a suggestion.
func recordedThisRun(res *botResult) bool {
	return res.Posted > 0 || len(res.Resolved) > 0 || len(res.Answered) > 0 || len(res.Suggested) > 0
}

// recordHead records the reviewed head in the status comment when the
// review posted nothing that records it. Failure is reported, not fatal:
// the next run then reviews more than needed, never less.
func recordHead(ctx context.Context, opts *options, rec *recalled, heads []string, res *botResult, stderr io.Writer) {
	head := opts.attr.Iteration
	if (res.Mode != scopeFull && res.Mode != scopeIncremental) || recordedThisRun(res) || slices.Contains(heads, head) {
		return
	}
	if !rec.listsSummaries {
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: mrman lists no pull-request-level comments (it predates review edit); "+
			"the reviewed head %.12s is not recorded\n", head)
		return
	}
	policyArgs := []string{"--grant-policy", opts.policyDir, "--garmr", opts.garmr, "--run-id", opts.attr.RunID, "--agent", agentName}
	if existing := statusComments(rec.summaries); len(existing) > 0 {
		s := existing[0]
		out, errOut, err := mrman(ctx, opts, stderr, append([]string{"review", "edit", "--session", rec.slug,
			"--summary", s.ID, statusBody(s.Body, head, opts.attr.RunID)}, policyArgs...)...)
		_, _ = stderr.Write(errOut)
		if err != nil {
			res.Status = "failed"
			_, _ = fmt.Fprintf(stderr, "vizzini-bot: the reviewed head %.12s is not recorded: mrman review edit: %s\n",
				head, statusFailure(out, errOut, "edit"))
			return
		}
		res.Status = "edited"
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: recorded the reviewed head %.12s in status comment %s\n", head, s.ID)
		return
	}
	if _, errOut, err := command(ctx, opts.repo, opts.mrman, "review", "add", "--session", rec.slug,
		statusBody("", head, opts.attr.RunID)); err != nil {
		res.Status = "failed"
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: the reviewed head %.12s is not recorded: mrman review add: %s\n",
			head, strings.TrimSpace(string(errOut)))
		return
	}
	out, errOut, err := mrman(ctx, opts, stderr, append([]string{"review", "submit", "--session", rec.slug,
		"--event", "comment"}, policyArgs...)...)
	_, _ = stderr.Write(errOut)
	if err != nil {
		res.Status = "failed"
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: the reviewed head %.12s is not recorded: mrman review submit: %s\n",
			head, statusFailure(out, errOut, "comment"))
		return
	}
	res.Status = "posted"
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: posted a status comment recording the reviewed head %.12s\n", head)
	if azureDevOps(opts.prURL) {
		resolveStatus(ctx, opts, rec.slug, policyArgs, stderr)
	}
}

// azureDevOps reports whether prURL is an Azure DevOps pull request.
func azureDevOps(prURL string) bool {
	u, err := url.Parse(prURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "dev.azure.com" || strings.HasSuffix(host, ".visualstudio.com")
}

// resolveStatus resolves the Azure DevOps thread that holds the status
// comment just posted, so it cannot block the pull request. Failure is
// logged: the thread then waits for a person, as any comment does.
func resolveStatus(ctx context.Context, opts *options, slug string, policyArgs []string, stderr io.Writer) {
	listed, errOut, err := mrman(ctx, opts, stderr, "review", "threads", "--session", slug)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: the status comment is not resolved: mrman review threads: %s\n", strings.TrimSpace(string(errOut)))
		return
	}
	var remote struct {
		Summaries []remoteSummary `json:"summaries"`
	}
	status := []remoteSummary{}
	if json.Unmarshal(listed, &remote) == nil {
		status = statusComments(remote.Summaries)
	}
	if len(status) == 0 {
		_, _ = fmt.Fprintln(stderr, "vizzini-bot: the status comment is not resolved: it is not listed yet")
		return
	}
	out, errOut, err := mrman(ctx, opts, stderr, append([]string{"review", "resolve", "--session", slug,
		"--thread", status[0].ID}, policyArgs...)...)
	_, _ = stderr.Write(errOut)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: the status comment is not resolved: mrman review resolve: %s\n",
			statusFailure(out, errOut, "resolve"))
		return
	}
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: resolved the status comment's thread %s\n", status[0].ID)
}

// statusFailure says why a status write failed, naming the policy event
// when the grant policy refused it.
func statusFailure(out, errOut []byte, event string) string {
	if policyDenied(out) {
		return fmt.Sprintf("the grant policy does not allow event %q (add it to _allowedEvents to record reviewed heads)", event)
	}
	return cmp.Or(strings.TrimSpace(string(out)), strings.TrimSpace(string(errOut)))
}
