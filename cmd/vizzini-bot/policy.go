package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The grant policy authorizes everything the bot posts (ADR-0016), so it
// must not come from the pull request it judges: a PR that edits the policy
// in its own checkout could allow itself to approve. A relative
// --grant-policy is therefore read from the BASE revision, like the
// suppression rules (ADR-0020): only a policy change that was reviewed and
// merged takes effect. An absolute path is the CI operator's own (baked into
// an image or written by the pipeline outside the repository) and is used as
// it is.

// basePolicy returns the policy directory to hand to mrman, and a cleanup.
func basePolicy(ctx context.Context, opts *options) (string, func(), error) {
	if filepath.IsAbs(opts.grantPolicy) {
		return opts.grantPolicy, func() {}, nil
	}
	rel := path.Clean(filepath.ToSlash(opts.grantPolicy))
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", nil, fmt.Errorf("--grant-policy %s: a relative policy must lie inside the repository", opts.grantPolicy)
	}
	out, errOut, err := command(ctx, opts.repo, "git", "ls-tree", "-r", "-z", "--name-only", opts.base, "--", rel)
	if err != nil {
		return "", nil, fmt.Errorf("--grant-policy: git ls-tree %s: %w: %s", opts.base, err, strings.TrimSpace(string(errOut)))
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if name := string(f); strings.HasSuffix(name, ".cue") {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("--grant-policy %s: no policy on %s; commit it to the base branch first "+
			"(a policy only takes effect once merged)", rel, opts.base)
	}
	dir, err := os.MkdirTemp("", "vizzini-bot-policy-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	for _, f := range files {
		data, gerr, serr := command(ctx, opts.repo, "git", "show", opts.base+":"+f)
		if serr != nil {
			cleanup()
			return "", nil, fmt.Errorf("--grant-policy: git show %s:%s: %w: %s", opts.base, f, serr, strings.TrimSpace(string(gerr)))
		}
		dst := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(f, rel+"/")))
		if f == rel { // the path named a single file
			dst = filepath.Join(dir, path.Base(f))
		}
		// Git tree paths cannot climb out, but the copy says so itself.
		if !strings.HasPrefix(dst, dir+string(filepath.Separator)) {
			cleanup()
			return "", nil, fmt.Errorf("--grant-policy: %s leaves the policy directory", f)
		}
		if merr := os.MkdirAll(filepath.Dir(dst), 0o700); merr != nil { //nolint:gosec // G703: dst is checked to lie inside dir
			cleanup()
			return "", nil, merr
		}
		if werr := os.WriteFile(dst, data, 0o600); werr != nil { //nolint:gosec // G703: dst is checked to lie inside dir
			cleanup()
			return "", nil, werr
		}
	}
	return dir, cleanup, nil
}
