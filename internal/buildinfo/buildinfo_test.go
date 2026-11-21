package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	old := [3]string{Version, Commit, Date}
	t.Cleanup(func() { Version, Commit, Date = old[0], old[1], old[2] })
	Version, Commit, Date = "v0.1.0", "abc123", "2026-11-21T00:00:00Z"

	got := String("pr-review")
	want := "pr-review v0.1.0 (commit abc123, built 2026-11-21T00:00:00Z, " + runtime.GOOS + "/" + runtime.GOARCH + ")"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestStringDefaults(t *testing.T) {
	if got := String("vizzini-bot"); !strings.HasPrefix(got, "vizzini-bot ") {
		t.Fatalf("String() = %q, want the name first", got)
	}
}

func TestRequested(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"--version"}, true},
		{[]string{"-version"}, true},
		{nil, false},
		{[]string{"--version", "--mode", "risk"}, false},
		{[]string{"--mode"}, false},
	} {
		if got := Requested(tc.args); got != tc.want {
			t.Errorf("Requested(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
