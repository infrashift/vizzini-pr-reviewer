package main

import (
	vmarker "github.com/infrashift/vizzini-pr-reviewer/pkg/marker"

	"bufio"
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
)

// Thread memory (ISSUE-024 slice 2). The bot keeps no store: what it said
// lives in the pull request's own threads, each carrying a hidden marker
//
//	<!-- vizzini:fp=v1:… run=… head=<sha> lines=<start>-<end> cat=<category> -->
//
// that records the finding's fingerprint and where it was anchored, at
// which head. A re-run lists the threads (mrman review threads), maps every
// earlier anchor to the current head through `git diff`, and then:
//
//   - skips a finding that matches a thread already there: the same
//     fingerprint, or the same file and category on overlapping lines once
//     mapped (a model rewords titles between runs, so the fingerprint alone
//     would repeat findings);
//   - resolves an open thread when no finding matches it any more AND the
//     code under its anchor changed since it was posted. A finding that
//     merely goes unreported while its code stands still (a model is not
//     deterministic) keeps its thread open;
//   - posts the rest.

// marker is what a posted comment remembers. Its format is pkg/marker's,
// which other tools (mrman's findings view) decode too.
type marker = vmarker.Marker

func parseMarker(body string) (marker, bool) { return vmarker.Parse(body) }

// remoteThread is the part of mrman's `review threads` output the bot reads.
type remoteThread struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Line       *int   `json:"line"`
	StartLine  *int   `json:"start_line"`
	IsResolved bool   `json:"is_resolved"`
	// Disposition is how it was resolved, where the forge says (Azure
	// DevOps: "resolved", "won't fix", "by design", "closed").
	Disposition string          `json:"disposition"`
	Comments    []remoteComment `json:"comments"`
}

type remoteComment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
}

// botThread is one of the bot's own earlier threads, its anchor mapped to
// the current head.
type botThread struct {
	ID       string
	Path     string
	Resolved bool
	Marker   marker
	// Anchor is where the code is now: anchorSame (mapped to Start..End),
	// anchorChanged (the code under it changed or went away) or
	// anchorUnknown (no head or lines recorded, or the old head is not in
	// this checkout).
	Anchor     anchorState
	Start, End int
}

type anchorState int

const (
	anchorUnknown anchorState = iota
	anchorSame
	anchorChanged
)

// lineMapper maps lines [start, end] of path at commit from to commit to.
type lineMapper func(ctx context.Context, path, from, to string, start, end int) (anchorState, int, int)

// botThreads picks the bot's threads (those with a marker in their first
// comment) and maps each to head.
func botThreads(ctx context.Context, threads []remoteThread, head string, mapLines lineMapper) []botThread {
	var out []botThread
	for i := range threads {
		t := &threads[i]
		if len(t.Comments) == 0 {
			continue
		}
		m, ok := parseMarker(t.Comments[0].Body)
		if !ok {
			continue
		}
		bt := botThread{ID: t.ID, Path: strings.TrimPrefix(t.Path, "/"), Resolved: t.IsResolved, Marker: m}
		switch {
		case m.Head == "" || m.Start <= 0 || m.End < m.Start:
			bt.Anchor = anchorUnknown
		case m.Head == head:
			bt.Anchor, bt.Start, bt.End = anchorSame, m.Start, m.End
		default:
			bt.Anchor, bt.Start, bt.End = mapLines(ctx, bt.Path, m.Head, head, m.Start, m.End)
		}
		out = append(out, bt)
	}
	return out
}

// reconciled is what a run does with its findings and earlier threads.
type reconciled struct {
	Post []comment
	// AlreadyPosted are the fingerprints of findings skipped because a
	// thread already raises them.
	AlreadyPosted []string
	// Resolve are open threads whose finding is gone and whose code changed.
	Resolve []string
	// KeptOpen are open threads whose finding was not reported this run
	// although their code did not change (or could not be mapped).
	KeptOpen []string
	// Carried are open threads on files this run did not review.
	Carried []string
}

// matches reports whether comment c raises what thread t raised.
func matches(c *comment, t *botThread) bool {
	if c.Fingerprint == t.Marker.FP {
		return true
	}
	return t.Anchor == anchorSame && c.File == t.Path && c.Category == t.Marker.Category &&
		c.StartLine <= t.End && c.EndLine >= t.Start
}

// reconcile decides what to post, skip and resolve. scope, when not nil, is
// the set of files this run reviewed: an open thread on any other file was
// not looked at, so it is carried as it is.
func reconcile(comments []comment, threads []botThread, scope map[string]bool) reconciled {
	var r reconciled
	matched := make([]bool, len(threads))
	for i := range comments {
		c := &comments[i]
		seen := false
		for j := range threads {
			t := &threads[j]
			// A thread resolved because its code changed was fixed: the
			// same finding again is a regression, worth a new thread.
			if t.Resolved && t.Anchor == anchorChanged {
				continue
			}
			if matches(c, t) {
				matched[j], seen = true, true
			}
		}
		switch {
		case c.suppressed:
			// Never posted; its threads are left alone (matched above).
		case seen:
			r.AlreadyPosted = append(r.AlreadyPosted, c.Fingerprint)
		default:
			r.Post = append(r.Post, *c)
		}
	}
	for j := range threads {
		t := &threads[j]
		if t.Resolved || matched[j] {
			continue
		}
		switch {
		case scope != nil && !scope[t.Path]:
			r.Carried = append(r.Carried, t.ID)
		case t.Anchor == anchorChanged:
			r.Resolve = append(r.Resolve, t.ID)
		default:
			r.KeptOpen = append(r.KeptOpen, t.ID)
		}
	}
	return r
}

// gitLineMapper maps an anchor through `git diff -U0 from to -- path` in
// repo. Any change touching the range, or the file's deletion, is
// anchorChanged; hunks above it shift it; an unknown commit is
// anchorUnknown (a shallow checkout cannot see the earlier head).
func gitLineMapper(repo string) lineMapper {
	return func(ctx context.Context, path, from, to string, start, end int) (anchorState, int, int) {
		out, _, err := command(ctx, repo, "git", "diff", "-U0", "--no-color", "--no-ext-diff", "--no-renames", from, to, "--", path)
		if err != nil {
			return anchorUnknown, 0, 0
		}
		return mapRange(out, start, end)
	}
}

var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// mapRange applies a -U0 diff of one file to the line range [start, end].
func mapRange(diff []byte, start, end int) (anchorState, int, int) {
	delta := 0
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "+++ /dev/null" || strings.HasPrefix(line, "deleted file mode") {
			return anchorChanged, 0, 0
		}
		h := hunkRE.FindStringSubmatch(line)
		if h == nil {
			continue
		}
		oldStart, _ := strconv.Atoi(h[1])
		oldCount, newCount := 1, 1
		if h[2] != "" {
			oldCount, _ = strconv.Atoi(h[2])
		}
		if h[4] != "" {
			newCount, _ = strconv.Atoi(h[4])
		}
		if oldCount == 0 {
			// Pure insertion after line oldStart.
			switch {
			case oldStart < start:
				delta += newCount
			case oldStart < end:
				return anchorChanged, 0, 0 // inserted inside the range
			}
			continue
		}
		oldEnd := oldStart + oldCount - 1
		switch {
		case oldEnd < start:
			delta += newCount - oldCount
		case oldStart <= end:
			return anchorChanged, 0, 0 // the range's own lines changed
		}
	}
	return anchorSame, start + delta, end + delta
}
