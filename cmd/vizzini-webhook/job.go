package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// commandFunc runs a program in dir with env and returns its output. Tests
// replace it.
type commandFunc func(ctx context.Context, dir string, env []string, name string, args ...string) (stdout, stderr []byte, err error)

func execCommand(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: running git and the configured vizzini-bot is the point
	cmd.Dir = dir
	cmd.Env = env
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	return o.Bytes(), e.Bytes(), err
}

// runner fetches a pull request into a cached clone and runs
// vizzini-bot --commands-only on it.
type runner struct {
	cfg     *config
	log     *slog.Logger
	command commandFunc
	mu      sync.Mutex
	locks   map[string]*sync.Mutex // repo key -> lock: one job per clone at a time
}

func (r *runner) repoLock(repo string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locks == nil {
		r.locks = map[string]*sync.Mutex{}
	}
	if r.locks[repo] == nil {
		r.locks[repo] = &sync.Mutex{}
	}
	return r.locks[repo]
}

// gitAuth hands git the token mrman also uses (AZURE_DEVOPS_EXT_PAT),
// through a credential helper that reads it from the environment: never on
// a command line.
var gitAuth = []string{"-c", "credential.helper=",
	"-c", `credential.helper=!f() { echo username=pat; echo "password=$AZURE_DEVOPS_EXT_PAT"; }; f`}

func (r *runner) run(ctx context.Context, j *job) {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.jobTimeout)
	defer cancel()
	log := r.log.With("pr", j.Key)
	mu := r.repoLock(j.Repo)
	mu.Lock()
	defer mu.Unlock()

	if err := r.answer(ctx, j, log); err != nil {
		log.Error("job failed", "error", err)
	}
}

func (r *runner) answer(ctx context.Context, j *job, log *slog.Logger) error {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	clone := filepath.Join(r.cfg.work, "repos", strings.ReplaceAll(j.Repo, "/", "_"))
	git := func(args ...string) (string, error) {
		out, errOut, err := r.command(ctx, clone, env, r.cfg.git, append(append([]string{}, gitAuth...), args...)...)
		if err != nil {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(errOut)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := os.Stat(filepath.Join(clone, ".git")); err != nil {
		if merr := os.MkdirAll(clone, 0o700); merr != nil {
			return merr
		}
		if _, cerr := git("clone", "--quiet", "--no-checkout", "--", j.CloneURL, "."); cerr != nil {
			_ = os.RemoveAll(clone)
			return cerr
		}
	}
	merge := fmt.Sprintf("refs/remotes/pull/%d/merge", j.PRID)
	if _, err := git("fetch", "--quiet", "--prune", "--force", "origin",
		fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", j.Target, j.Target),
		fmt.Sprintf("+refs/pull/%d/merge:%s", j.PRID, merge)); err != nil {
		return err
	}
	// The PR merge ref's second parent is the source head, also for a PR
	// from a fork.
	head, err := git("rev-parse", "--verify", merge+"^2")
	if err != nil {
		return err
	}
	if _, err = git("checkout", "--quiet", "--force", "--detach", head); err != nil {
		return err
	}
	if _, err = git("clean", "-ffdxq"); err != nil {
		return err
	}

	// Each job gets its own mrman data directory, as each CI run has.
	data, err := os.MkdirTemp(r.cfg.work, "job-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(data) }()
	args := []string{
		"--repo", clone,
		"--base", "origin/" + j.Target,
		"--head", head,
		"--pr-url", j.WebURL,
		"--grant-policy", r.cfg.grantPolicy,
		"--pr", j.Key,
		"--requester", j.Requester,
		"--commands-only",
		"--max-commands", strconv.Itoa(r.cfg.maxCommands),
		"--timeout", r.cfg.jobTimeout.String(),
	}
	if r.cfg.commandAuthor != "" {
		args = append(args, "--command-authors", r.cfg.commandAuthor)
	}
	args = append(args, r.cfg.botArgs...)
	log.Info("job started", "head", head[:min(12, len(head))], "requester", redact(j.Requester))
	out, errOut, err := r.command(ctx, clone, append(env, "XDG_DATA_HOME="+data), r.cfg.bot, args...)
	var res struct {
		Answered        []string `json:"answered"`
		CommandFailures []string `json:"command_failures"`
	}
	_ = json.Unmarshal(lastLine(out), &res)
	if err != nil {
		code := -1
		if ee, ok := err.(interface{ ExitCode() int }); ok {
			code = ee.ExitCode()
		}
		if code == 3 {
			if id := deniedIdentity(errOut); id != "" {
				return fmt.Errorf("the grant policy denied the reply (vizzini-bot exit 3): add %q to _allowedIdentities in the base branch's policy", id)
			}
			return errors.New("the grant policy denied the reply (vizzini-bot exit 3): list this receiver's identity in the base branch's policy")
		}
		return fmt.Errorf("vizzini-bot exit %d: %s", code, tail(errOut, 600))
	}
	log.Info("job done", "answered", res.Answered, "failed", res.CommandFailures)
	return nil
}

func lastLine(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return b
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = "…" + s[len(s)-n:]
	}
	return s
}

// redact keeps a person's identity out of the logs: three characters.
func redact(s string) string {
	r := []rune(s)
	if len(r) <= 3 {
		return "…"
	}
	return string(r[:3]) + "…"
}

// deniedIdentity is the identity mrman's denied authorization names: the
// account this receiver's token posts as.
func deniedIdentity(stderr []byte) string {
	for _, line := range strings.Split(string(stderr), "\n") {
		_, rest, ok := strings.Cut(line, "mrman: authorization: ")
		if !ok {
			continue
		}
		var d struct {
			Input struct {
				Identity string `json:"identity"`
			} `json:"input"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(rest)), &d) == nil && d.Input.Identity != "" {
			return d.Input.Identity
		}
	}
	return ""
}
