package reviewer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/infrashift/vizzini-pr-reviewer/internal/chunker"
	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/llm"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// CommandKind names a slash command.
type CommandKind string

// Slash commands available in a review thread (Milestone 4.6).
const (
	CommandExplain    CommandKind = "explain"
	CommandRefactor   CommandKind = "refactor"
	CommandDocstrings CommandKind = "docstrings"
)

// Command is a parsed slash command.
type Command struct {
	Kind CommandKind
	File string
	// StartLine and EndLine are new-file lines; both are 0 for /docstrings,
	// which covers every changed hunk of File.
	StartLine, EndLine int
	// Question is any free text after the target.
	Question string
}

// ThreadMessage is one earlier comment in the review thread.
type ThreadMessage = llm.ThreadMessage

// CommandResult is the validated answer to a command.
type CommandResult struct {
	Command Command `json:"command"`
	// Answer is the Markdown explanation (/explain).
	Answer string `json:"answer,omitempty"`
	// Suggestions are anchored to the command's selection (/refactor,
	// /docstrings).
	Suggestions []CodeSuggestion `json:"suggestions,omitempty"`
}

// ErrBadCommand marks input that is not a valid slash command.
var ErrBadCommand = errors.New("reviewer: bad command")

// maxThreadTokens bounds the thread history sent with a command; the most
// recent messages are kept.
const maxThreadTokens = 1500

// ParseCommand parses "/explain FILE:START[-END] [question]",
// "/refactor FILE:START[-END] [question]" or "/docstrings FILE [note]".
// Lines may be written GitHub-style as L44 or L44-L45.
func ParseCommand(s string) (*Command, error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return nil, fmt.Errorf("%w: commands start with /explain, /refactor or /docstrings", ErrBadCommand)
	}
	cmd := &Command{Kind: CommandKind(strings.ToLower(strings.TrimPrefix(fields[0], "/")))}
	switch cmd.Kind {
	case CommandExplain, CommandRefactor, CommandDocstrings:
	default:
		return nil, fmt.Errorf("%w: unknown command %s (use /explain, /refactor or /docstrings)", ErrBadCommand, fields[0])
	}
	if len(fields) < 2 {
		return nil, fmt.Errorf("%w: /%s needs a target", ErrBadCommand, cmd.Kind)
	}
	target := fields[1]
	cmd.Question = strings.Join(fields[2:], " ")

	if cmd.Kind == CommandDocstrings {
		if _, rng, ok := cutRange(target); ok && rng != "" {
			return nil, fmt.Errorf("%w: /docstrings takes a file, not a line range", ErrBadCommand)
		}
		cmd.File = target
		return cmd, nil
	}

	file, rng, ok := cutRange(target)
	if !ok {
		return nil, fmt.Errorf("%w: /%s needs FILE:LINE or FILE:START-END, got %q", ErrBadCommand, cmd.Kind, target)
	}
	start, end, err := parseRange(rng)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrBadCommand, target, err)
	}
	cmd.File, cmd.StartLine, cmd.EndLine = file, start, end
	return cmd, nil
}

// cutRange splits "path:range" at the last colon whose suffix looks like a
// line range; paths containing colons are left intact otherwise.
func cutRange(target string) (file, rng string, ok bool) {
	i := strings.LastIndexByte(target, ':')
	if i <= 0 || i == len(target)-1 {
		return target, "", false
	}
	r := target[i+1:]
	if r[0] != 'L' && (r[0] < '0' || r[0] > '9') {
		return target, "", false
	}
	return target[:i], r, true
}

func parseRange(r string) (start, end int, err error) {
	a, b, isRange := strings.Cut(r, "-")
	if start, err = parseLine(a); err != nil {
		return 0, 0, err
	}
	end = start
	if isRange {
		if end, err = parseLine(b); err != nil {
			return 0, 0, err
		}
	}
	if end < start {
		return 0, 0, fmt.Errorf("range ends (%d) before it starts (%d)", end, start)
	}
	return start, end, nil
}

func parseLine(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(s, "L"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("bad line number %q", s)
	}
	return n, nil
}

// String renders the command's target as "file:start-end" or "file".
func (c *Command) String() string {
	switch c.StartLine {
	case 0:
		return c.File
	case c.EndLine:
		return fmt.Sprintf("%s:%d", c.File, c.StartLine)
	default:
		return fmt.Sprintf("%s:%d-%d", c.File, c.StartLine, c.EndLine)
	}
}

// RunCommand answers a slash command about patch. Only the hunks of
// cmd.File that overlap the selection (every hunk, for /docstrings) and the
// thread history are sent to the model; suggestions must stay inside that
// scope.
func (e *Engine) RunCommand(ctx context.Context, patch io.Reader, cmd *Command, thread []ThreadMessage) (*CommandResult, error) {
	ctx, _, _ = begin(ctx, "command")
	d, err := e.parse(patch)
	if err != nil {
		return nil, err
	}
	scope, err := scopeFor(d, cmd)
	if err != nil {
		return nil, err
	}
	p, err := llm.Render(string(cmd.Kind), llm.CommandData{
		Target: cmd.String(), Question: cmd.Question, Diff: scope.Render(), Thread: e.recentThread(thread),
	})
	if err != nil {
		return nil, err
	}

	res := &CommandResult{Command: *cmd}
	if cmd.Kind == CommandExplain {
		_, err = e.llm.CompleteJSON(ctx, p, func(b []byte) error {
			x, verr := e.validator.ValidateExplanation(b)
			if verr == nil {
				res.Answer = x.Answer
			}
			return verr
		})
		return res, err
	}

	a := newAnchors(scope.Parts)
	_, err = e.llm.CompleteJSON(ctx, p, func(b []byte) error {
		r, verr := e.validator.ValidateImprove(b)
		if verr != nil {
			return verr
		}
		for j := range r.Suggestions {
			s := &r.Suggestions[j]
			if aerr := a.check("suggestions", j, s.File, s.StartLine, s.EndLine); aerr != nil {
				return aerr
			}
			// A refactor must stay inside the selection, not merely touch it.
			if cmd.Kind == CommandRefactor && (s.StartLine < cmd.StartLine || s.EndLine > cmd.EndLine) {
				return fmt.Errorf("%w: suggestions.%d: lines %d-%d reach outside the selection %s; refactor only lines %d-%d",
					schema.ErrInvalid, j, s.StartLine, s.EndLine, cmd, cmd.StartLine, cmd.EndLine)
			}
		}
		res.Suggestions = r.Suggestions
		return nil
	})
	if res.Suggestions == nil {
		res.Suggestions = []CodeSuggestion{}
	}
	return res, err
}

// scopeFor builds a single-part chunk holding only the hunks of cmd.File the
// command is about.
func scopeFor(d *diff.Diff, cmd *Command) (*chunker.Chunk, error) {
	for _, f := range d.Files {
		if f.Path != cmd.File {
			continue
		}
		var hunks []diff.Hunk
		for _, h := range f.Hunks {
			start := int(h.Fragment.NewPosition)
			end := start + max(int(h.Fragment.NewLines), 1) - 1
			if cmd.StartLine == 0 || (cmd.StartLine <= end && cmd.EndLine >= start) {
				hunks = append(hunks, h)
			}
		}
		if len(hunks) == 0 {
			return nil, fmt.Errorf("%w: %s has no changed lines in this diff", ErrBadCommand, cmd)
		}
		return &chunker.Chunk{Parts: []chunker.Part{{File: f, Hunks: hunks}}}, nil
	}
	return nil, fmt.Errorf("%w: %s is not part of this diff", ErrBadCommand, cmd.File)
}

// recentThread keeps the newest messages that fit maxThreadTokens.
func (e *Engine) recentThread(thread []ThreadMessage) []ThreadMessage {
	used, first := 0, len(thread)
	for i := len(thread) - 1; i >= 0; i-- {
		t := e.chunker.CountTokens(thread[i].Author + ": " + thread[i].Body)
		if used+t > maxThreadTokens {
			break
		}
		used += t
		first = i
	}
	return thread[first:]
}
