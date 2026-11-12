package marker

import (
	"reflect"
	"strings"
	"testing"
)

func TestMarkerRoundTrip(t *testing.T) {
	m := Marker{FP: "v1:abc", Run: "r1", Head: strings.Repeat("ab", 20), Category: "Security", Start: 12, End: 14}
	got, ok := Parse("text\n\n" + m.String())
	if !ok || got != m {
		t.Fatalf("%+v %v", got, ok)
	}
	// The first format had no head, lines or category.
	old, ok := Parse("<!-- vizzini:fp=v1:x run=r0 -->")
	if !ok || old != (Marker{FP: "v1:x", Run: "r0"}) {
		t.Errorf("slice-1 marker: %+v %v", old, ok)
	}
	if _, ok := Parse("no marker"); ok {
		t.Error("parsed a body without a marker")
	}
}

func TestCommentRoundTrip(t *testing.T) {
	for name, c := range map[string]Comment{
		"full": {Severity: "Major", Category: "Security", Title: "SQL built by concatenation",
			Body: "The name is concatenated.\n\nAn attacker controls it.", Suggestion: "db.Query(\"… WHERE name = ?\", name)",
			Citations: []string{"01a1cae2", "ADR-0001"}, Agent: "vizzini-bot",
			Marker: Marker{FP: "v1:abc", Run: "r1", Head: strings.Repeat("cd", 20), Category: "Security", Start: 3, End: 3}},
		"bare": {Severity: "Minor", Category: "Logic", Title: "t", Body: "b", Agent: "vizzini-bot",
			Marker: Marker{FP: "v1:z", Run: "r", Head: strings.Repeat("ef", 20), Category: "Logic", Start: 1, End: 2}},
		"suggestion only": {Severity: "Trivial", Category: "Style", Title: "t", Body: "", Suggestion: "x := 1", Agent: "vizzini-bot",
			Marker: Marker{FP: "v1:q", Run: "r", Head: strings.Repeat("01", 20), Category: "Style", Start: 5, End: 5}},
	} {
		got, ok := ParseComment(c.Render())
		if !ok || !reflect.DeepEqual(got, c) {
			t.Errorf("%s:\n got %+v\nwant %+v\nbody:\n%s", name, got, c, c.Render())
		}
	}
	// CRLF from a forge, and a hand-edited body, still give the marker.
	c := Comment{Severity: "Major", Category: "Logic", Title: "t", Body: "b", Agent: "vizzini-bot",
		Marker: Marker{FP: "v1:a", Run: "r", Head: strings.Repeat("aa", 20), Category: "Logic", Start: 1, End: 1}}
	if got, ok := ParseComment(strings.ReplaceAll(c.Render(), "\n", "\r\n")); !ok || got.Title != "t" || got.Body != "b" {
		t.Errorf("CRLF: %+v", got)
	}
	if got, ok := ParseComment("free text\n" + c.Marker.String()); !ok || got.Marker != c.Marker || got.Category != "Logic" {
		t.Errorf("hand-edited: %+v", got)
	}
	if _, ok := ParseComment("no marker"); ok {
		t.Error("parsed a comment without a marker")
	}
}

func TestReplyToAndFromVizzini(t *testing.T) {
	if ReplyTo("answer\n\n<!-- vizzini:reply-to=42 run=r head=abcdef0 -->") != "42" || ReplyTo("x") != "" {
		t.Error("ReplyTo")
	}
	if !FromVizzini("<!-- vizzini:reviewed head=a run=b -->") || FromVizzini("plain") {
		t.Error("FromVizzini")
	}
}
