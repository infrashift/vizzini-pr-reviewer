package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBasePolicy(t *testing.T) {
	dir, git, write := scopeRepo(t) // main: a.go, b.go; feature changes both
	git("checkout", "-q", "main")
	if err := os.MkdirAll(filepath.Join(dir, "ci"), 0o750); err != nil {
		t.Fatal(err)
	}
	write("ci/policy.cue", "package mrman // base\n")
	git("add", "ci")
	git("commit", "-q", "-m", "policy")
	git("checkout", "-q", "feature")

	// An absolute path is the operator's and is used as is.
	if got, cleanup, err := basePolicy(t.Context(), &options{repo: dir, base: "main", grantPolicy: "/etc/vizzini/policy"}); err != nil ||
		got != "/etc/vizzini/policy" {
		t.Errorf("absolute: %q %v", got, err)
	} else {
		cleanup()
	}
	// A single policy file, from the base.
	got, cleanup, err := basePolicy(t.Context(), &options{repo: dir, base: "main", grantPolicy: "ci/policy.cue"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(got, "policy.cue")) //nolint:gosec // G304: the function's own temp copy
	if !strings.Contains(string(data), "// base") {
		t.Errorf("single file: %q", data)
	}
	cleanup()
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Error("cleanup must remove the copy")
	}
	for name, p := range map[string]string{
		"not on the base":        "policies/mrman",
		"outside the repository": "../elsewhere",
		"the repository itself":  ".",
	} {
		if _, _, err := basePolicy(t.Context(), &options{repo: dir, base: "main", grantPolicy: p}); err == nil {
			t.Errorf("%s: accepted %q", name, p)
		}
	}
}
