package main

import (
	"strings"
	"testing"
)

// A Trivial finding is below the default noise floor (ISSUE-032): not shown,
// counted in the output, and shown again with --all-findings.
func TestRun_NoiseFloorAndAllFindings(t *testing.T) {
	srv := reviewWith(t, "Trivial")
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}

	r := runCLI(t, env, patch)
	if r.code != exitOK || strings.Contains(r.stdout, "`internal/store/users.go:42`") || !strings.Contains(r.stdout, "1 lower-priority finding(s) below the noise floor") {
		t.Errorf("default: exit %d, stdout:\n%s", r.code, r.stdout)
	}

	r = runCLI(t, env, patch, "--format", "json")
	if r.code != exitOK || !strings.Contains(r.stdout, `"filtered"`) {
		t.Errorf("JSON must list the filtered finding: %s", r.stdout)
	}

	r = runCLI(t, env, patch, "--all-findings")
	if r.code != exitOK || strings.Contains(r.stdout, "below the noise floor") || !strings.Contains(r.stdout, "`internal/store/users.go:42`") {
		t.Errorf("--all-findings: exit %d, stdout:\n%s", r.code, r.stdout)
	}
}

// The floor decides what is shown, never what blocks.
func TestRun_GateSeesFindingsBelowTheFloor(t *testing.T) {
	srv := reviewWith(t, "Trivial")
	env := map[string]string{"VIZZINI_LITELLM_URL": srv.BaseURL(), "VIZZINI_MODEL": "m"}
	r := runCLI(t, env, patch, "--fail-on", "Trivial")
	if r.code != exitPolicy || !strings.Contains(r.stderr, "1 of them are below the noise floor and not shown; --all-findings shows them") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	r = runCLI(t, env, patch, "--fail-on", "Trivial", "--all-findings")
	if r.code != exitPolicy || strings.Contains(r.stderr, "below the noise floor") {
		t.Errorf("with --all-findings: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestRun_AllFindingsOnlyInReviewMode(t *testing.T) {
	r := runCLI(t, map[string]string{"VIZZINI_LITELLM_URL": "http://127.0.0.1:1", "VIZZINI_MODEL": "m"}, patch, "--mode", "describe", "--all-findings")
	if r.code != exitError || !strings.Contains(r.stderr, "--all-findings applies only to --mode review") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}
