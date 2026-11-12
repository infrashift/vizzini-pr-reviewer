package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

// Suppression (ISSUE-024 slice 5, ADR-0020). The repository's rules are
// read from the BASE revision, never the pull request's head: a rule takes
// effect once it is merged, so an author cannot silence findings on their
// own change by adding a rule to it. A finding a rule matches is not
// posted, and a thread it matches is left alone.
//
// When a person resolves one of the bot's threads as won't fix or by
// design, the bot answers once with a rule they can commit to stop the
// finding coming back; the rule takes effect only when that commit merges.

// loadSuppressions reads opts.suppressions at the base revision. A missing
// file means no rules; an unreadable or invalid one fails the run, because
// rules silently ignored would post what a team decided not to see.
func loadSuppressions(ctx context.Context, opts *options, stderr io.Writer) (*reviewer.Suppressions, error) {
	if opts.suppressions == "" {
		return nil, nil
	}
	spec := opts.base + ":" + strings.TrimPrefix(opts.suppressions, "./")
	data, errOut, err := command(ctx, opts.repo, "git", "show", spec)
	if err != nil {
		if bytes.Contains(errOut, []byte("does not exist")) || bytes.Contains(errOut, []byte("exists on disk, but not in")) {
			return nil, nil
		}
		return nil, fmt.Errorf("--suppressions: git show %s: %w: %s", spec, err, strings.TrimSpace(string(errOut)))
	}
	rules, err := reviewer.ParseSuppressions(data)
	if err != nil {
		return nil, fmt.Errorf("--suppressions %s: %w", spec, err)
	}
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: %d suppression rule(s) from %s\n", rules.Len(), spec)
	return rules, nil
}

// suppressedComments are the comments suppressed findings would have been:
// never posted, but still matched against the bot's threads so a thread
// raising a now-suppressed finding is left alone.
func suppressedComments(suppressed []reviewer.SuppressedFinding) []comment {
	out := make([]comment, 0, len(suppressed))
	for i := range suppressed {
		f := &suppressed[i].Finding
		out = append(out, comment{File: f.File, Category: string(f.Category), StartLine: f.StartLine, EndLine: f.EndLine,
			Fingerprint: reviewer.Fingerprint(f), suppressed: true})
	}
	return out
}

// filteredComments are findings below the noise floor (ISSUE-032), held
// back like suppressed ones: a finding the bot posted before, and still
// sees, must not have its thread resolved as fixed because the floor now
// keeps it from being posted.
func filteredComments(filtered []reviewer.Finding) []comment {
	out := make([]comment, 0, len(filtered))
	for i := range filtered {
		f := &filtered[i]
		out = append(out, comment{File: f.File, Category: string(f.Category), StartLine: f.StartLine, EndLine: f.EndLine,
			Fingerprint: reviewer.Fingerprint(f), suppressed: true})
	}
	return out
}

var (
	suggestionMarkerRE = regexp.MustCompile(`<!-- vizzini:suggested-suppression `)
	findingHeadingRE   = regexp.MustCompile(`^\*\*\w+ \((\w+)\): (.+)\*\*`)
)

// dismissals are the bot's threads a person resolved as won't fix or by
// design that no suggestion answers yet.
func dismissals(threads []remoteThread) []*remoteThread {
	var out []*remoteThread
	for i := range threads {
		t := &threads[i]
		if !t.IsResolved || (t.Disposition != "won't fix" && t.Disposition != "by design") || len(t.Comments) == 0 {
			continue
		}
		if _, ok := parseMarker(t.Comments[0].Body); !ok {
			continue // not the bot's
		}
		if !slices.ContainsFunc(t.Comments, func(c remoteComment) bool { return suggestionMarkerRE.MatchString(c.Body) }) {
			out = append(out, t)
		}
	}
	return out
}

// suggestion is the reply proposing a rule for a dismissed thread.
func suggestion(t *remoteThread, opts *options) string {
	m, _ := parseMarker(t.Comments[0].Body)
	category, title := m.Category, ""
	if h := findingHeadingRE.FindStringSubmatch(strings.TrimSpace(t.Comments[0].Body)); h != nil {
		category, title = cmp.Or(category, h[1]), h[2]
	}
	id := strings.ToLower(cmp.Or(category, "finding")) + "-" + strings.TrimPrefix(m.FP, "v1:")[:8]
	where := cmp.Or(opts.attr.PRKey, "this pull request")
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) } // a JSON string is a YAML string
	var b strings.Builder
	fmt.Fprintf(&b, "This finding was dismissed (%s). To stop Vizzini raising it again in this repository, "+
		"commit a rule like this to `%s` on the base branch:\n\n```yaml\nsuppressions:\n", t.Disposition, opts.suppressions)
	fmt.Fprintf(&b, "  - id: %s\n    reason: %s\n    paths: [%s]\n", id, q("Dismissed as "+t.Disposition+" in "+where+": say why"), q(t.Path))
	if category != "" {
		fmt.Fprintf(&b, "    categories: [%s]\n", category)
	}
	if title != "" {
		fmt.Fprintf(&b, "    # Narrower, but a model may word the title differently next time:\n    # title: %s\n", q(title))
	}
	fmt.Fprintf(&b, "```\n\nThe rule covers %s findings in this file. It takes effect once merged: the bot reads rules from the base branch.\n\n"+
		"_Suggested by %s._\n\n<!-- vizzini:suggested-suppression thread=%s run=%s head=%s -->",
		cmp.Or(category, "all"), agentName, t.ID, opts.attr.RunID, opts.attr.Iteration)
	return b.String()
}

// suggestSuppressions answers each new dismissal once.
func suggestSuppressions(ctx context.Context, opts *options, slug, policy string, threads []remoteThread,
	res *botResult, stderr io.Writer) error {
	for _, t := range dismissals(threads) {
		if err := reply(ctx, opts, slug, policy, t.ID, t.Comments[0].ID, suggestion(t, opts), stderr); err != nil {
			if errors.Is(err, errDenied) {
				return err
			}
			_, _ = fmt.Fprintf(stderr, "vizzini-bot: suppression suggestion in thread %s failed: %v\n", t.ID, err)
			continue
		}
		res.Suggested = append(res.Suggested, t.ID)
	}
	return nil
}
