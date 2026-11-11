package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

// In-thread commands (ISSUE-024 slice 4). Anyone who can comment on the
// pull request can write, in any file-anchored thread,
//
//	/vizzini explain [FILE:START-END] [question]
//	/vizzini refactor [FILE:START-END] [what to change]
//	/vizzini docstrings [FILE] [note]
//
// and the next bot run answers in that thread. Without a target the
// command is about the thread's own lines (or file, for /docstrings). The
// thread's earlier comments travel with it as conversation. The reply
// carries <!-- vizzini:reply-to=<comment id> run=… -->, which is how a later
// run knows the command was answered: each is answered once. A command that
// cannot be understood is answered once with how to write it; a failure to
// reach the model is not answered, so the next run tries again.

var (
	// commandLineRE finds a /vizzini command on a line of its own.
	commandLineRE = regexp.MustCompile(`(?im)^[ \t]*/vizzini[ \t]+(\S[^\r\n]*)$`)
	replyMarkerRE = regexp.MustCompile(`<!-- vizzini:reply-to=(\S+) run=\S+(?: head=[0-9a-f]+)? -->`)
	// explicitTargetRE is a FILE:LINES target as ParseCommand reads it.
	explicitTargetRE = regexp.MustCompile(`^\S+:L?\d+(?:-L?\d+)?$`)
)

// pendingCommand is a /vizzini command no bot reply answers yet.
type pendingCommand struct {
	Thread  *remoteThread
	Comment remoteComment
	Text    string // after "/vizzini "
	// History is the thread's comments before the command.
	History []reviewer.ThreadMessage
}

// pendingCommands lists unanswered commands, oldest thread first. Comments
// that carry a vizzini marker are the bot's own and are never commands.
func pendingCommands(threads []remoteThread, authors []string) []pendingCommand {
	var out []pendingCommand
	for i := range threads {
		t := &threads[i]
		answered := map[string]bool{}
		for _, c := range t.Comments {
			for _, m := range replyMarkerRE.FindAllStringSubmatch(c.Body, -1) {
				answered[m[1]] = true
			}
		}
		var history []reviewer.ThreadMessage
		for _, c := range t.Comments {
			mine := strings.Contains(c.Body, "<!-- vizzini:")
			if m := commandLineRE.FindStringSubmatch(c.Body); m != nil && !mine && !answered[c.ID] &&
				(len(authors) == 0 || slices.Contains(authors, c.Author)) {
				out = append(out, pendingCommand{Thread: t, Comment: c, Text: strings.TrimSpace(m[1]),
					History: slices.Clone(history)})
			}
			history = append(history, reviewer.ThreadMessage{Author: c.Author, Body: c.Body})
		}
	}
	return out
}

// threadCommand turns "explain why …" in thread t into a reviewer command,
// filling in the thread's own target when none is named.
func threadCommand(text string, t *remoteThread) (*reviewer.Command, error) {
	fields := strings.Fields(text)
	kind := strings.ToLower(fields[0])
	rest := fields[1:]
	explicit := len(rest) > 0 && (explicitTargetRE.MatchString(rest[0]) ||
		(kind == string(reviewer.CommandDocstrings) && rest[0] == t.Path))
	if !explicit {
		var target string
		switch {
		case kind == string(reviewer.CommandDocstrings):
			target = t.Path
		case t.Line == nil:
			return nil, fmt.Errorf("%w: this thread is not anchored to lines any more; name them, e.g. /vizzini %s %s:12-20",
				reviewer.ErrBadCommand, kind, t.Path)
		default:
			start := *t.Line
			if t.StartLine != nil {
				start = *t.StartLine
			}
			target = fmt.Sprintf("%s:%d-%d", t.Path, start, *t.Line)
		}
		rest = append([]string{target}, rest...)
	}
	return reviewer.ParseCommand("/" + kind + " " + strings.Join(rest, " "))
}

// commandUsage is the reply to a command that could not be understood.
func commandUsage(err error) string {
	msg := strings.TrimPrefix(err.Error(), reviewer.ErrBadCommand.Error()+": ")
	return "I could not run that: " + msg + "\n\n" +
		"Write one of these on a line of its own; without a target it is about this thread's lines:\n\n" +
		"```\n/vizzini explain [FILE:START-END] [question]\n/vizzini refactor [FILE:START-END] [what to change]\n" +
		"/vizzini docstrings [FILE] [note]\n```"
}

// answerCommands runs and answers up to opts.maxCommands pending commands.
// diff is the pull request's diff (base...head). A policy denial stops and
// is returned; other failures are reported and left for the next run.
func answerCommands(ctx context.Context, opts *options, slug, policy string, diff []byte, threads []remoteThread,
	engine *reviewer.Engine, res *botResult, stderr io.Writer) error {
	pending := pendingCommands(threads, opts.commandAuthors)
	if len(pending) == 0 {
		return nil
	}
	if len(pending) > opts.maxCommands {
		_, _ = fmt.Fprintf(stderr, "vizzini-bot: %d commands waiting; answering %d this run\n", len(pending), opts.maxCommands)
		pending = pending[:opts.maxCommands]
	}
	for i := range pending {
		p := &pending[i]
		var body string
		cmd, err := threadCommand(p.Text, p.Thread)
		if err == nil {
			var out *reviewer.CommandResult
			out, err = engine.RunCommand(ctx, bytes.NewReader(diff), cmd, p.History)
			if err == nil {
				body = reviewer.FormatCommandResult(out)
			}
		}
		switch {
		case errors.Is(err, reviewer.ErrBadCommand): // includes a target outside this diff
			body = commandUsage(err)
		case err != nil:
			_, _ = fmt.Fprintf(stderr, "vizzini-bot: command in thread %s not answered (next run retries): %v\n", p.Thread.ID, err)
			res.CommandFailures = append(res.CommandFailures, p.Comment.ID)
			continue
		}
		body += fmt.Sprintf("\n\n_Answered by %s._\n\n<!-- vizzini:reply-to=%s run=%s head=%s -->",
			agentName, p.Comment.ID, opts.attr.RunID, opts.attr.Iteration)
		if err := reply(ctx, opts, slug, policy, p.Thread.ID, p.Comment.ID, body, stderr); err != nil {
			if errors.Is(err, errDenied) {
				return err
			}
			_, _ = fmt.Fprintf(stderr, "vizzini-bot: reply in thread %s failed: %v\n", p.Thread.ID, err)
			res.CommandFailures = append(res.CommandFailures, p.Comment.ID)
			continue
		}
		res.Answered = append(res.Answered, p.Comment.ID)
	}
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: answered %d of %d commands\n", len(res.Answered), len(pending))
	return nil
}

// reply posts body under comment parent in thread through mrman. The body
// goes through a file, so no reply can be mistaken for a flag.
func reply(ctx context.Context, opts *options, slug, policy, thread, parent, body string, stderr io.Writer) error {
	dir, err := os.MkdirTemp("", "vizzini-bot-reply-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	file := filepath.Join(dir, "reply.md")
	if werr := os.WriteFile(file, []byte(body), 0o600); werr != nil {
		return werr
	}
	out, errOut, err := mrman(ctx, opts, stderr, "review", "reply", "--session", slug, "--thread", thread,
		"--parent", parent, "--input", "@"+file, "--grant-policy", policy, "--garmr", opts.garmr,
		"--run-id", opts.attr.RunID, "--agent", agentName)
	_, _ = stderr.Write(errOut)
	if err != nil {
		if policyDenied(out) {
			return errDenied
		}
		return fmt.Errorf("mrman review reply: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}

// parseAuthors splits a comma-separated --command-authors value.
func parseAuthors(s string) []string {
	var out []string
	for a := range strings.SplitSeq(s, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}
