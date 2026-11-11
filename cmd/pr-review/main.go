// Command pr-review runs a Vizzini review of a unified diff from the command
// line, for CI jobs, UBI9 containers and QEMU microVMs.
//
// Exit codes: 0 success, 1 error (including usage errors), 2 the review has
// findings at a severity listed in --fail-on (default Critical and Major).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

const (
	exitOK     = 0
	exitError  = 1
	exitPolicy = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(code)
}

const usage = `Usage: pr-review [flags]

Review a unified diff with an LLM behind a LiteLLM proxy. Every answer is
validated against Vizzini's CUE schema before it is printed.

Examples:
  git diff main...HEAD | pr-review --mode review
  pr-review --patch-file change.patch --mode describe --format json

Exit codes:
  0  success
  1  error
  2  review mode: a finding's severity is listed in --fail-on

Environment:
  VIZZINI_LITELLM_URL   default for --litellm-url
  VIZZINI_MODEL         default for --model
  VIZZINI_API_KEY       bearer token for the proxy (falls back to LITELLM_API_KEY)
  TIKTOKEN_CACHE_DIR    pre-seeded tokenizer cache for offline use

Flags:
`

type options struct {
	patchFile      string
	mode           string
	litellmURL     string
	model          string
	format         string
	maxChunkTokens int
	concurrency    int
	timeout        time.Duration
	skipOversized  bool
	// failOn is nil unless --fail-on was given; nil selects the schema default.
	failOn     []reviewer.Severity
	lintJSON   string
	riskPolicy string
	knowledge  string
	command    string
	thread     string
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	opts, err := parseFlags(args, stderr, getenv)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	var blocking []reviewer.Finding
	var gate *reviewer.PolicyGate
	if err == nil {
		blocking, gate, err = execute(ctx, opts, stdin, stdout, getenv)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "pr-review:", err)
		return exitError
	}
	if len(blocking) > 0 {
		_, _ = fmt.Fprintf(stderr, "pr-review: policy gate failed: %d finding(s) at %s\n", len(blocking), joinSeverities(gate.FailOn))
		return exitPolicy
	}
	return exitOK
}

func joinSeverities(s []reviewer.Severity) string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		out = append(out, string(x))
	}
	return strings.Join(out, ", ")
}

// execute runs the mode and writes its output. In review mode it returns the
// findings that fail the policy gate, and the gate itself.
func execute(ctx context.Context, opts *options, stdin io.Reader, stdout io.Writer, getenv func(string) string) ([]reviewer.Finding, *reviewer.PolicyGate, error) {
	apiKey := getenv("VIZZINI_API_KEY")
	if apiKey == "" {
		apiKey = getenv("LITELLM_API_KEY")
	}
	var riskPolicy []byte
	if opts.riskPolicy != "" {
		data, rerr := os.ReadFile(opts.riskPolicy) //nolint:gosec // G703: reading the user-named policy file is the point
		if rerr != nil {
			return nil, nil, fmt.Errorf("--risk-policy: %w", rerr)
		}
		riskPolicy = data
	}
	engine, err := reviewer.New(reviewer.Config{
		RiskPolicy:     riskPolicy,
		BaseURL:        opts.litellmURL,
		APIKey:         apiKey,
		Model:          opts.model,
		MaxChunkTokens: opts.maxChunkTokens,
		Concurrency:    opts.concurrency,
	})
	if err != nil {
		return nil, nil, err
	}

	// Validate the gate before spending tokens on a review.
	var gate *reviewer.PolicyGate
	if opts.mode == "review" {
		if gate, err = engine.Policy(opts.failOn); err != nil {
			return nil, nil, fmt.Errorf("--fail-on: %w", err)
		}
	}

	patch, closePatch, err := openPatch(opts.patchFile, stdin)
	if err != nil {
		return nil, nil, err
	}
	defer closePatch()

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	result, text, err := dispatch(ctx, engine, opts, patch)
	if err != nil {
		return nil, nil, err
	}
	if opts.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(result)
	} else {
		_, err = io.WriteString(stdout, text)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("write output: %w", err)
	}
	if r, ok := result.(*reviewer.ReviewOutput); ok && gate != nil {
		blocking := gate.Blocking(r.Findings)
		return blocking, gate, nil
	}
	return nil, nil, nil
}

// dispatch runs the selected mode and returns its JSON value and Markdown.
func dispatch(ctx context.Context, engine *reviewer.Engine, opts *options, patch io.Reader) (result any, markdown string, err error) {
	switch opts.mode {
	case "describe":
		d, derr := engine.Describe(ctx, patch)
		if derr != nil {
			return nil, "", derr
		}
		return d, reviewer.FormatDescription(d), nil
	case "command":
		cmd, cerr := reviewer.ParseCommand(opts.command)
		if cerr != nil {
			return nil, "", cerr
		}
		var thread []reviewer.ThreadMessage
		if opts.thread != "" {
			data, terr := os.ReadFile(opts.thread) //nolint:gosec // G304: reading the user-named thread file is the point
			if terr != nil {
				return nil, "", fmt.Errorf("--thread: %w", terr)
			}
			if terr = json.Unmarshal(data, &thread); terr != nil {
				return nil, "", fmt.Errorf("--thread: %w", terr)
			}
		}
		res, rerr := engine.RunCommand(ctx, patch, cmd, thread)
		if rerr != nil {
			return nil, "", rerr
		}
		return res, reviewer.FormatCommandResult(res), nil
	case "improve":
		s, ierr := engine.Improve(ctx, patch)
		if ierr != nil {
			return nil, "", ierr
		}
		return map[string]any{"suggestions": s}, reviewer.FormatSuggestions(s), nil
	default: // "review"; parseFlags rejects anything else
		ropts := reviewer.ReviewOptions{SkipOversized: opts.skipOversized}
		if opts.knowledge != "" {
			st, serr := os.Stat(opts.knowledge) //nolint:gosec // G703: inspecting the user-named knowledge directory is the point
			if serr != nil {
				return nil, "", fmt.Errorf("--knowledge: %w", serr)
			}
			if !st.IsDir() {
				return nil, "", fmt.Errorf("--knowledge: %s is not a directory", opts.knowledge)
			}
			ropts.Knowledge = os.DirFS(opts.knowledge)
		}
		if opts.lintJSON != "" {
			lint, closeLint, lerr := openPatch(opts.lintJSON, nil)
			if lerr != nil {
				return nil, "", fmt.Errorf("--lint-json: %w", lerr)
			}
			defer closeLint()
			ropts.LintReport = lint
		}
		r, rerr := engine.Review(ctx, patch, ropts)
		if rerr != nil {
			return nil, "", rerr
		}
		return r, reviewer.FormatReview(r), nil
	}
}

func parseFlags(args []string, stderr io.Writer, getenv func(string) string) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("pr-review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&o.patchFile, "patch-file", "-", "unified diff to review; - reads stdin")
	fs.StringVar(&o.mode, "mode", "review", "describe | review | improve | command")
	fs.StringVar(&o.command, "command", "", `command mode: slash command, e.g. "/explain internal/store/users.go:44-45 why?"`)
	fs.StringVar(&o.thread, "thread", "", "command mode: JSON file with the thread history, [{\"author\": ..., \"body\": ...}]")
	fs.StringVar(&o.litellmURL, "litellm-url", getenv("VIZZINI_LITELLM_URL"), "OpenAI-compatible API root of the LiteLLM proxy, e.g. http://litellm:4000/v1")
	fs.StringVar(&o.model, "model", getenv("VIZZINI_MODEL"), "model name as configured in LiteLLM")
	fs.StringVar(&o.format, "format", "markdown", "json | markdown")
	fs.IntVar(&o.maxChunkTokens, "max-chunk-tokens", 0, "token budget per LLM request (default 6000)")
	fs.IntVar(&o.concurrency, "concurrency", 0, "parallel LLM requests (default 4)")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Minute, "overall deadline")
	fs.BoolVar(&o.skipOversized, "skip-oversized", false, "review mode: skip hunks larger than the token budget")
	fs.StringVar(&o.riskPolicy, "risk-policy", "", "JSON risk policy (#RiskPolicy in schema/review.cue): path weights, scale, auto_approve_threshold")
	fs.StringVar(&o.knowledge, "knowledge", "", "review mode: OKF knowledge-base directory (e.g. .okfw) whose relevant documents the model may cite")
	fs.StringVar(&o.lintJSON, "lint-json", "", "review mode: golangci-lint JSON report (run from the repo root) to explain on changed lines")
	fs.Func("fail-on", "review mode: comma-separated severities that exit 2, or none (default Critical,Major)", func(s string) error {
		o.failOn = []reviewer.Severity{}
		if s == "none" {
			return nil
		}
		for _, part := range strings.Split(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				o.failOn = append(o.failOn, reviewer.Severity(part))
			}
		}
		return nil
	})

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	switch o.mode {
	case "describe", "review", "improve", "command":
	default:
		return nil, fmt.Errorf("--mode must be describe, review, improve or command, got %q", o.mode)
	}
	if (o.mode == "command") != (o.command != "") {
		return nil, errors.New("--command and --mode command go together")
	}
	if o.thread != "" && o.mode != "command" {
		return nil, errors.New("--thread applies only to --mode command")
	}
	if o.failOn != nil && o.mode != "review" {
		return nil, errors.New("--fail-on applies only to --mode review")
	}
	if o.lintJSON != "" && o.mode != "review" {
		return nil, errors.New("--lint-json applies only to --mode review")
	}
	if o.knowledge != "" && o.mode != "review" {
		return nil, errors.New("--knowledge applies only to --mode review")
	}
	if o.lintJSON == "-" {
		return nil, errors.New("--lint-json must be a file; stdin is reserved for the patch")
	}
	switch o.format {
	case "json", "markdown":
	default:
		return nil, fmt.Errorf("--format must be json or markdown, got %q", o.format)
	}
	if o.litellmURL == "" {
		return nil, errors.New("--litellm-url (or VIZZINI_LITELLM_URL) is required")
	}
	if o.model == "" {
		return nil, errors.New("--model (or VIZZINI_MODEL) is required")
	}
	if o.timeout <= 0 {
		return nil, errors.New("--timeout must be positive")
	}
	return o, nil
}

func openPatch(name string, stdin io.Reader) (io.Reader, func(), error) {
	if name == "-" {
		return stdin, func() {}, nil
	}
	f, err := os.Open(name) //nolint:gosec // G304: reading the user-named patch file is the point
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}
