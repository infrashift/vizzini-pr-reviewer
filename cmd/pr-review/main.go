// Command pr-review runs a Vizzini review of a unified diff from the command
// line, for CI jobs, UBI9 containers and QEMU microVMs.
//
// Exit codes: 0 success, 1 error (including usage errors). Exit code 2 is
// reserved for the severity policy gate.
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
	"syscall"
	"time"

	"github.com/infrashift/vizzini-pr-reviewer/pkg/reviewer"
)

const (
	exitOK    = 0
	exitError = 1
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
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	opts, err := parseFlags(args, stderr, getenv)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err == nil {
		err = execute(ctx, opts, stdin, stdout, getenv)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "pr-review:", err)
		return exitError
	}
	return exitOK
}

func execute(ctx context.Context, opts *options, stdin io.Reader, stdout io.Writer, getenv func(string) string) error {
	apiKey := getenv("VIZZINI_API_KEY")
	if apiKey == "" {
		apiKey = getenv("LITELLM_API_KEY")
	}
	engine, err := reviewer.New(reviewer.Config{
		BaseURL:        opts.litellmURL,
		APIKey:         apiKey,
		Model:          opts.model,
		MaxChunkTokens: opts.maxChunkTokens,
		Concurrency:    opts.concurrency,
	})
	if err != nil {
		return err
	}

	patch, closePatch, err := openPatch(opts.patchFile, stdin)
	if err != nil {
		return err
	}
	defer closePatch()

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	result, text, err := dispatch(ctx, engine, opts, patch)
	if err != nil {
		return err
	}
	if opts.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(result)
	} else {
		_, err = io.WriteString(stdout, text)
	}
	if err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
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
	case "improve":
		s, ierr := engine.Improve(ctx, patch)
		if ierr != nil {
			return nil, "", ierr
		}
		return map[string]any{"suggestions": s}, reviewer.FormatSuggestions(s), nil
	default: // "review"; parseFlags rejects anything else
		r, rerr := engine.Review(ctx, patch, reviewer.ReviewOptions{SkipOversized: opts.skipOversized})
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
	fs.StringVar(&o.mode, "mode", "review", "describe | review | improve")
	fs.StringVar(&o.litellmURL, "litellm-url", getenv("VIZZINI_LITELLM_URL"), "OpenAI-compatible API root of the LiteLLM proxy, e.g. http://litellm:4000/v1")
	fs.StringVar(&o.model, "model", getenv("VIZZINI_MODEL"), "model name as configured in LiteLLM")
	fs.StringVar(&o.format, "format", "markdown", "json | markdown")
	fs.IntVar(&o.maxChunkTokens, "max-chunk-tokens", 0, "token budget per LLM request (default 6000)")
	fs.IntVar(&o.concurrency, "concurrency", 0, "parallel LLM requests (default 4)")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Minute, "overall deadline")
	fs.BoolVar(&o.skipOversized, "skip-oversized", false, "review mode: skip hunks larger than the token budget")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	switch o.mode {
	case "describe", "review", "improve":
	default:
		return nil, fmt.Errorf("--mode must be describe, review or improve, got %q", o.mode)
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
