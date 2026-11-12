// Package marker is the format of the comments vizzini-bot posts: a finding
// rendered for people, and a hidden HTML-comment marker that lets later runs,
// and other tools, recognize the finding (Vizzini ISSUE-024, ISSUE-027).
//
// It has no dependencies beyond the standard library, so forge tools such as
// mrman can decode the bot's threads without importing the reviewer.
//
// A finding comment looks like this:
//
//	**Major (Security): SQL built by concatenation**
//
//	The name is concatenated into the query.
//
//	Suggestion:
//	```
//	db.Query("… WHERE name = ?", name)
//	```
//
//	See: 01a1cae2-6830-71f7-b6b5-4ab70d034cdb
//
//	_Posted by vizzini-bot._
//
//	<!-- vizzini:fp=v1:… run=… head=<sha> lines=12-14 cat=Security -->
package marker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Marker is what a posted finding remembers: its fingerprint, the run that
// posted it, and where it was anchored at which head.
type Marker struct {
	FP       string `json:"fingerprint"`
	Run      string `json:"run"`
	Head     string `json:"head,omitempty"`
	Category string `json:"category,omitempty"`
	Start    int    `json:"start_line,omitempty"`
	End      int    `json:"end_line,omitempty"`
}

// markerRE parses a marker; head, lines and cat are absent from the first
// format (ISSUE-024 slice 1).
var markerRE = regexp.MustCompile(`<!-- vizzini:fp=(\S+) run=(\S+)(?: head=([0-9a-f]{7,64}))?(?: lines=(\d+)-(\d+))?(?: cat=(\S+))? -->`)

// String renders the marker as the hidden HTML comment.
func (m *Marker) String() string {
	return fmt.Sprintf("<!-- vizzini:fp=%s run=%s head=%s lines=%d-%d cat=%s -->", m.FP, m.Run, m.Head, m.Start, m.End, m.Category)
}

// Parse finds the finding marker in a comment body.
func Parse(body string) (Marker, bool) {
	sub := markerRE.FindStringSubmatch(body)
	if sub == nil {
		return Marker{}, false
	}
	m := Marker{FP: sub[1], Run: sub[2], Head: sub[3], Category: sub[6]}
	m.Start, _ = strconv.Atoi(sub[4])
	m.End, _ = strconv.Atoi(sub[5])
	return m, true
}

// FromVizzini reports whether a comment was written by vizzini-bot: every
// comment it posts carries a "<!-- vizzini:" marker.
func FromVizzini(body string) bool { return strings.Contains(body, "<!-- vizzini:") }

// Comment is a finding as the bot posts it.
type Comment struct {
	Severity   string   `json:"severity"`
	Category   string   `json:"category"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Suggestion string   `json:"suggestion,omitempty"`
	Citations  []string `json:"citations,omitempty"`
	// Agent is the tool that posted it ("vizzini-bot").
	Agent  string `json:"agent,omitempty"`
	Marker Marker `json:"marker"`
}

// Render is the comment body.
func (c *Comment) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s (%s): %s**\n\n%s", c.Severity, c.Category, c.Title, strings.TrimSpace(c.Body))
	if s := strings.TrimSpace(c.Suggestion); s != "" {
		fmt.Fprintf(&b, "\n\nSuggestion:\n```\n%s\n```", s)
	}
	if len(c.Citations) > 0 {
		fmt.Fprintf(&b, "\n\nSee: %s", strings.Join(c.Citations, ", "))
	}
	fmt.Fprintf(&b, "\n\n_Posted by %s._\n\n%s", c.Agent, c.Marker.String())
	return b.String()
}

var (
	headRE   = regexp.MustCompile(`^\*\*(\S+) \(([^)]*)\): (.*)\*\*$`)
	postedRE = regexp.MustCompile(`(?s)\n\n_Posted by ([^_\n]*)\._\n\n<!-- vizzini:fp=.*$`)
)

// ParseComment decodes a finding comment. ok is false when body carries no
// finding marker. A body whose prose does not follow the layout (an older
// or hand-edited comment) still yields its marker, with what could be read.
func ParseComment(body string) (c Comment, ok bool) {
	m, ok := Parse(body)
	if !ok {
		return Comment{}, false
	}
	c.Marker = m
	c.Category = m.Category
	text := strings.ReplaceAll(body, "\r\n", "\n")
	if loc := postedRE.FindStringSubmatchIndex(text); loc != nil {
		c.Agent = text[loc[2]:loc[3]]
		text = text[:loc[0]]
	}
	first, rest, _ := strings.Cut(text, "\n\n")
	if h := headRE.FindStringSubmatch(strings.TrimSpace(first)); h != nil {
		c.Severity, c.Title = h[1], h[3]
		if c.Category == "" {
			c.Category = h[2]
		}
		text = rest
	}
	if i := strings.LastIndex(text, "\n\nSee: "); i >= 0 {
		for _, s := range strings.Split(text[i+len("\n\nSee: "):], ",") {
			if s = strings.TrimSpace(s); s != "" {
				c.Citations = append(c.Citations, s)
			}
		}
		text = text[:i]
	}
	if i := strings.Index(text, "\n\nSuggestion:\n```\n"); i >= 0 {
		sug := text[i+len("\n\nSuggestion:\n```\n"):]
		sug = strings.TrimSuffix(strings.TrimRight(sug, "\n"), "```")
		c.Suggestion = strings.TrimSpace(sug)
		text = text[:i]
	} else if strings.HasPrefix(text, "Suggestion:\n```\n") {
		c.Suggestion = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "Suggestion:\n```\n"), "```"))
		text = ""
	}
	c.Body = strings.TrimSpace(text)
	return c, true
}

// replyRE matches the marker on a bot reply that answers a command.
var replyRE = regexp.MustCompile(`<!-- vizzini:reply-to=(\S+) run=\S+(?: head=[0-9a-f]+)? -->`)

// ReplyTo is the comment id a bot reply answers ("" when body is not one).
func ReplyTo(body string) string {
	if m := replyRE.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	return ""
}
