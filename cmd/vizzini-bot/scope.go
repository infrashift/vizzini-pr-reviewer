package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Incremental review (ISSUE-024 slice 3). Every comment the bot posts
// records the head its run reviewed: a finding's marker (head=), a
// resolution's and a command answer's (<!-- vizzini:reviewed head=… --> and
// reply-to … head=…). The last reviewed head is the newest of those that is
// an ancestor of the current head. A push is then
// reviewed by its own changes: the pull request's diff (base...head),
// restricted to the files that changed since that head. Changes merged in
// from the base branch are not in base...head and are never reviewed; a
// head that did not move is not reviewed at all. Threads on files left out
// are carried as they are.
//
// A run that posts nothing at all (no finding, no resolution, no answer)
// leaves no trace, so the next run reviews from the last head that did:
// more than needed, never less.
//
// With no earlier head in this checkout (a first run, a rebased or
// force-pushed branch, a shallow clone) or with --full, the whole pull
// request is reviewed.

const (
	scopeFull        = "full"
	scopeIncremental = "incremental"
	scopeUnchanged   = "unchanged"
)

type reviewScopeT struct {
	mode  string
	since string
	files []string
}

// scope returns the reviewed files as a set, or nil for a full review.
func (s *reviewScopeT) scope() map[string]bool {
	if s.mode != scopeIncremental {
		return nil
	}
	m := make(map[string]bool, len(s.files))
	for _, f := range s.files {
		m[f] = true
	}
	return m
}

// headMarkerRE finds the head recorded in any of the bot's markers.
var headMarkerRE = regexp.MustCompile(`<!-- vizzini:[^>]*? head=([0-9a-f]{7,64})[^>]*-->`)

// reviewedHeads lists every head the bot's comments record, in any thread.
func reviewedHeads(threads []remoteThread) []string {
	var heads []string
	for i := range threads {
		for _, c := range threads[i].Comments {
			for _, m := range headMarkerRE.FindAllStringSubmatch(c.Body, -1) {
				if !slices.Contains(heads, m[1]) {
					heads = append(heads, m[1])
				}
			}
		}
	}
	return heads
}

func reviewScope(ctx context.Context, opts *options, heads []string) (*reviewScopeT, error) {
	full := &reviewScopeT{mode: scopeFull}
	if opts.full || len(heads) == 0 {
		return full, nil
	}
	since := lastReviewedHead(ctx, opts.repo, opts.attr.Iteration, heads)
	if since == "" {
		return full, nil
	}
	if since == opts.attr.Iteration {
		return &reviewScopeT{mode: scopeUnchanged, since: since}, nil
	}
	changed, err := changedFiles(ctx, opts.repo, since, opts.attr.Iteration)
	if err != nil {
		return nil, err
	}
	inPR, err := changedFiles(ctx, opts.repo, opts.base+"..."+opts.attr.Iteration)
	if err != nil {
		return nil, err
	}
	sc := &reviewScopeT{mode: scopeIncremental, since: since}
	for _, f := range inPR {
		if slices.Contains(changed, f) {
			sc.files = append(sc.files, f)
		}
	}
	if len(sc.files) == 0 {
		// The push changed nothing the pull request changes (say, a merge
		// from the base branch).
		sc.mode = scopeUnchanged
	}
	return sc, nil
}

// lastReviewedHead is the recorded head closest to head among those that
// are its ancestors (or head itself); "" when none is.
func lastReviewedHead(ctx context.Context, repo, head string, heads []string) string {
	best, bestDistance := "", -1
	for _, h := range heads {
		if h == head {
			return head
		}
		if _, _, err := command(ctx, repo, "git", "merge-base", "--is-ancestor", h, head); err != nil {
			continue // not an ancestor, or not in this checkout
		}
		out, _, err := command(ctx, repo, "git", "rev-list", "--count", h+".."+head)
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			continue
		}
		if bestDistance < 0 || n < bestDistance {
			best, bestDistance = h, n
		}
	}
	return best
}

// changedFiles lists the paths a diff touches (no rename detection, so a
// rename lists both paths).
func changedFiles(ctx context.Context, repo string, revs ...string) ([]string, error) {
	args := append([]string{"diff", "--name-only", "--no-renames", "-z"}, revs...)
	out, errOut, err := command(ctx, repo, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s: %w: %s", strings.Join(revs, " "), err, strings.TrimSpace(string(errOut)))
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) > 0 {
			files = append(files, string(f))
		}
	}
	return files, nil
}
