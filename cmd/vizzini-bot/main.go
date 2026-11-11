// Command vizzini-bot reviews a pull request from a CI checkout and posts
// the findings to the forge without a person at a terminal (ADR-0016,
// ISSUE-024).
//
// It composes three tools, each with one job:
//
//   - Vizzini (this module's library) reviews `git diff base...head`, with
//     the checkout as repository context and attribution for cost logs.
//   - mrman does all forge work: `mrman pr URL --json` opens the PR
//     headlessly, `mrman review add` drafts one comment per finding, and
//     `mrman review submit --grant-policy DIR` posts them.
//   - garmr, run by mrman, decides whether the submit is allowed by the
//     committed policy in DIR. The bot never authorizes itself: a deny (or
//     an evaluation error) posts nothing.
//
// Exit codes: 0 posted (or nothing to post), 1 error, 3 denied by the grant
// policy.
package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

const (
	exitOK     = 0
	exitError  = 1
	exitDenied = 3

	agentName = "vizzini-bot"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(code)
}

type options struct {
	prURL       string
	base, head  string
	repo        string
	knowledge   string
	lintJSON    []string
	litellmURL  string
	model       string
	grantPolicy string
	mrman       string
	garmr       string
	out         string
	dryRun      bool
	timeout     time.Duration
	attr        reviewer.Attribution
}

func parseFlags(args []string, stderr io.Writer, getenv func(string) string) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("vizzini-bot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.prURL, "pr-url", getenv("VIZZINI_PR_URL"), "the pull request's web URL, as mrman opens it (required)")
	fs.StringVar(&o.base, "base", getenv("VIZZINI_BASE"), "the target branch or commit; the review covers base...head (required)")
	fs.StringVar(&o.head, "head", cmp.Or(getenv("VIZZINI_HEAD"), "HEAD"), "the PR's head commit")
	fs.StringVar(&o.repo, "repo", ".", "the CI checkout: the diff's source and the review's repository context")
	fs.StringVar(&o.knowledge, "knowledge", "", "an OKF wiki directory to ground the review in (optional)")
	fs.Func("lint-json", "a linter report (golangci-lint JSON or SARIF) produced from the checkout, explained on changed lines (repeatable)", func(s string) error {
		o.lintJSON = append(o.lintJSON, s)
		return nil
	})
	fs.StringVar(&o.litellmURL, "litellm-url", getenv("VIZZINI_LITELLM_URL"), "OpenAI-compatible API root of the LiteLLM proxy")
	fs.StringVar(&o.model, "model", getenv("VIZZINI_MODEL"), "model name as configured in LiteLLM")
	fs.StringVar(&o.grantPolicy, "grant-policy", getenv("VIZZINI_GRANT_POLICY"), "the committed garmr policy directory that authorizes posting (required unless --dry-run)")
	fs.StringVar(&o.mrman, "mrman", cmp.Or(getenv("VIZZINI_MRMAN"), "mrman"), "the mrman binary")
	fs.StringVar(&o.garmr, "garmr", cmp.Or(getenv("VIZZINI_GARMR"), "garmr"), "the garmr binary mrman evaluates the policy with")
	fs.StringVar(&o.out, "out", "", "also write the review report (JSON) here")
	fs.BoolVar(&o.dryRun, "dry-run", false, "review and print the comments it would post; touch no forge")
	fs.DurationVar(&o.timeout, "timeout", 15*time.Minute, "overall time limit")
	fs.StringVar(&o.attr.RunID, "run-id", getenv("VIZZINI_RUN_ID"), "this run's id (default: a generated UUIDv7)")
	fs.StringVar(&o.attr.PRKey, "pr", getenv("VIZZINI_PR"), `attribution: the pull request key, e.g. "ado:dev.azure.com/org/project/repo#42"`)
	fs.StringVar(&o.attr.Requester, "requester", getenv("VIZZINI_REQUESTER"), "attribution: the human the run is for (e.g. the PR author)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	switch {
	case o.base == "":
		return nil, errors.New("--base is required")
	case o.prURL == "" && !o.dryRun:
		return nil, errors.New("--pr-url is required")
	case o.grantPolicy == "" && !o.dryRun:
		return nil, errors.New("--grant-policy is required: posting is authorized only by a committed policy")
	}
	o.attr.Agent = agentName
	if o.attr.RunID == "" {
		o.attr.RunID = reviewer.NewRunID()
	}
	return o, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	opts, err := parseFlags(args, stderr, getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		_, _ = fmt.Fprintln(stderr, "vizzini-bot:", err)
		return exitError
	}
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	if err := botRun(ctx, opts, stdout, stderr, getenv); err != nil {
		_, _ = fmt.Fprintln(stderr, "vizzini-bot:", err)
		if errors.Is(err, errDenied) {
			return exitDenied
		}
		return exitError
	}
	return exitOK
}

var errDenied = errors.New("the grant policy denied posting; nothing was posted")

// command runs an external tool (git, mrman) and returns its stdout. Tests
// replace it.
var command = func(ctx context.Context, dir, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: running git and the configured mrman is the point
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.Bytes(), e.Bytes(), err
}

func botRun(ctx context.Context, opts *options, stdout, stderr io.Writer, getenv func(string) string) error {
	head, _, err := command(ctx, opts.repo, "git", "rev-parse", "--verify", opts.head+"^{commit}")
	if err != nil {
		return fmt.Errorf("--head %s: %w", opts.head, err)
	}
	opts.attr.Iteration = strings.TrimSpace(string(head))
	diff, gitErr, err := command(ctx, opts.repo, "git", "diff", "--no-color", "--no-ext-diff", opts.base+"..."+opts.attr.Iteration)
	if err != nil {
		return fmt.Errorf("git diff %s...%s: %w: %s", opts.base, opts.head, err, strings.TrimSpace(string(gitErr)))
	}
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: run %s reviewing %s...%.12s\n", opts.attr.RunID, opts.base, opts.attr.Iteration)

	review, err := reviewDiff(ctx, opts, diff, getenv)
	if err != nil {
		return err
	}
	comments := plan(review.Findings, opts.attr.RunID)
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: %d findings, risk %d\n", len(comments), review.RiskScore)

	if opts.dryRun {
		enc := json.NewEncoder(stdout)
		for i := range comments {
			if err := enc.Encode(&comments[i]); err != nil {
				return err
			}
		}
		return nil
	}
	if len(comments) == 0 {
		_, _ = fmt.Fprintln(stderr, "vizzini-bot: nothing to post")
		return nil
	}
	return post(ctx, opts, comments, stdout, stderr)
}

// reviewDiff runs the Vizzini review and, with --out, writes its report.
func reviewDiff(ctx context.Context, opts *options, diff []byte, getenv func(string) string) (*reviewer.ReviewOutput, error) {
	engine, err := reviewer.New(reviewer.Config{
		BaseURL: opts.litellmURL,
		APIKey:  cmp.Or(getenv("VIZZINI_API_KEY"), getenv("LITELLM_API_KEY")),
		Model:   opts.model,
	})
	if err != nil {
		return nil, err
	}
	ropts := reviewer.ReviewOptions{Repo: os.DirFS(opts.repo)}
	if opts.knowledge != "" {
		ropts.Knowledge = os.DirFS(opts.knowledge)
	}
	for _, name := range opts.lintJSON {
		data, rerr := os.ReadFile(name) //nolint:gosec // G304: reading the user-named lint report is the point
		if rerr != nil {
			return nil, fmt.Errorf("--lint-json: %w", rerr)
		}
		ropts.LintReports = append(ropts.LintReports, bytes.NewReader(data))
	}
	if len(ropts.LintReports) > 0 {
		// Reports come from the checkout's root; SARIF file:// URIs resolve against it.
		root, aerr := filepath.Abs(opts.repo)
		if aerr != nil {
			return nil, aerr
		}
		ropts.LintRoot = root
	}
	rec := &reviewer.RunRecorder{}
	ctx = reviewer.WithRunRecorder(reviewer.WithAttribution(ctx, opts.attr), rec)
	review, err := engine.Review(ctx, bytes.NewReader(diff), ropts)
	if err != nil {
		return nil, fmt.Errorf("review: %w", err)
	}
	if opts.out != "" {
		var info *reviewer.RunInfo
		if runs := rec.Runs(); len(runs) > 0 {
			info = &runs[len(runs)-1]
		}
		report, merr := reviewer.MarshalReportWithRun(review, info)
		if merr != nil {
			return nil, merr
		}
		if werr := os.WriteFile(opts.out, report, 0o600); werr != nil { //nolint:gosec // G703: writing the user-named report file is the point
			return nil, fmt.Errorf("--out: %w", werr)
		}
	}
	return review, nil
}

// comment is one finding as the bot posts it.
type comment struct {
	File        string `json:"file"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Fingerprint string `json:"fingerprint"`
	Body        string `json:"body"`
}

// plan turns findings into comments, each carrying a hidden marker with its
// fingerprint and the run that posted it, so later runs can recognize their
// own threads (ISSUE-024 slice 2).
func plan(findings []reviewer.Finding, runID string) []comment {
	out := make([]comment, 0, len(findings))
	for i := range findings {
		f := &findings[i]
		fp := fingerprint(f)
		var b strings.Builder
		fmt.Fprintf(&b, "**%s (%s): %s**\n\n%s", f.Severity, f.Category, f.Title, strings.TrimSpace(f.Body))
		if s := strings.TrimSpace(f.Suggestion); s != "" {
			fmt.Fprintf(&b, "\n\nSuggestion:\n```\n%s\n```", s)
		}
		if len(f.Citations) > 0 {
			fmt.Fprintf(&b, "\n\nSee: %s", strings.Join(f.Citations, ", "))
		}
		fmt.Fprintf(&b, "\n\n_Posted by %s._\n\n<!-- vizzini:fp=%s run=%s -->", agentName, fp, runID)
		out = append(out, comment{File: f.File, StartLine: f.StartLine, EndLine: f.EndLine, Fingerprint: fp, Body: b.String()})
	}
	return out
}

// fingerprint identifies a finding independently of its line numbers, so a
// finding that moves when code above it changes keeps its identity: the
// file, the category and the normalized title. "v1" versions the recipe.
func fingerprint(f *reviewer.Finding) string {
	title := strings.Join(strings.Fields(strings.ToLower(f.Title)), " ")
	sum := sha256.Sum256([]byte(f.File + "\x00" + string(f.Category) + "\x00" + title))
	return "v1:" + hex.EncodeToString(sum[:8])
}

// post opens the PR in mrman, drafts every comment, and submits a comment
// review that mrman posts only if the grant policy allows it.
func post(ctx context.Context, opts *options, comments []comment, stdout, stderr io.Writer) error {
	opened, errOut, err := command(ctx, opts.repo, opts.mrman, "pr", opts.prURL, "--json")
	if err != nil {
		return fmt.Errorf("mrman pr: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	var session struct {
		Slug string `json:"slug"`
	}
	if jerr := json.Unmarshal(opened, &session); jerr != nil || session.Slug == "" {
		return errors.New("mrman pr: no session slug in its output")
	}
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: mrman session %s\n", session.Slug)

	added := 0
	for i := range comments {
		c := &comments[i]
		args := []string{"review", "add", "--session", session.Slug, "--target-file", c.File,
			"--line", strconv.Itoa(c.StartLine), "--side", "new"}
		if c.EndLine > c.StartLine {
			args = append(args, "--end-line", strconv.Itoa(c.EndLine))
		}
		if _, e, aerr := command(ctx, opts.repo, opts.mrman, append(args, c.Body)...); aerr != nil {
			// One unplaceable comment must not lose the rest.
			_, _ = fmt.Fprintf(stderr, "vizzini-bot: skipped %s:%d: %s\n", c.File, c.StartLine, strings.TrimSpace(string(e)))
			continue
		}
		added++
	}
	if added == 0 {
		return errors.New("mrman accepted none of the comments")
	}

	policy, err := filepath.Abs(opts.grantPolicy)
	if err != nil {
		return err
	}
	res, errOut, err := command(ctx, opts.repo, opts.mrman, "review", "submit", "--session", session.Slug,
		"--event", "comment", "--grant-policy", policy, "--garmr", opts.garmr,
		"--run-id", opts.attr.RunID, "--agent", agentName)
	// mrman prints the authorization on stderr; keep it in the CI log.
	_, _ = stderr.Write(errOut)
	_, _ = stdout.Write(res)
	if err != nil {
		var denied struct {
			Code string `json:"error"`
		}
		if json.Unmarshal(res, &denied) == nil && denied.Code == "policy_denied" {
			return errDenied
		}
		return fmt.Errorf("mrman review submit: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "vizzini-bot: posted %d of %d comments\n", added, len(comments))
	return nil
}
