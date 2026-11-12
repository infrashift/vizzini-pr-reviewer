package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

// End to end (ISSUE-032): a finding below the noise floor is not posted, and
// the open thread that raised it on an earlier run is left alone instead of
// being resolved as fixed.
func TestBot_FilteredFindingIsNotPostedAndItsThreadIsLeftAlone(t *testing.T) {
	dir := suppressRepo(t, "suppressions: []\n")
	srv := mockFindings(t, `{"file":"a.go","start_line":3,"end_line":3,"severity":"Minor","category":"Maintainability","title":"Extract A","body":"b"},`+
		`{"file":"b.go","start_line":3,"end_line":3,"severity":"Minor","category":"Logic","title":"Bad B","body":"b"}`)
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	fp := reviewer.Fingerprint(&reviewer.Finding{File: "a.go", Category: "Maintainability", Title: "Extract A"})
	m := marker{FP: fp, Run: "r0", Head: strings.Repeat("ab", 20), Category: "Maintainability", Start: 3, End: 3}
	threads, _ := json.Marshal(map[string]any{"threads": []any{map[string]any{
		"id": "31", "path": "a.go", "comments": []any{map[string]any{"id": "1", "body": "x\n\n" + m.String()}}}}})
	f := &fakeMrman{submitOut: `{}`, threadsOut: string(threads)}
	f.install(t)
	code, stdout, stderr := runBot(t, env, "--repo", dir, "--base", "main", "--pr-url", "u", "--grant-policy", "/p", "--full")
	var res botResult
	if err := json.Unmarshal([]byte(stdout), &res); code != exitOK || err != nil {
		t.Fatalf("exit %d, %s: %s", code, stdout, stderr)
	}
	if res.Filtered != 1 || res.Posted != 1 || len(res.Resolved) != 0 || len(res.KeptOpen) != 0 || len(res.AlreadyPosted) != 0 {
		t.Errorf("result %s", stdout)
	}
	if !strings.Contains(stderr, "1 below the noise floor") {
		t.Errorf("stderr %q", stderr)
	}
	for _, c := range f.calls {
		if c[1] == "add" && slices.Contains(c, "a.go") {
			t.Errorf("a filtered finding was drafted: %v", c)
		}
	}
}
