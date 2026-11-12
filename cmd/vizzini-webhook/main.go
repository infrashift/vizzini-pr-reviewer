// Command vizzini-webhook answers /vizzini commands as soon as they are
// written, instead of on the pull request's next CI run. EXPERIMENTAL and
// optional: without it, everything works as before and commands are
// answered by the next vizzini-bot run (ADR-0022, ISSUE-026).
//
// An Azure DevOps service hook ("Pull request commented on", Web Hooks)
// posts each new comment here. The receiver
//
//   - authenticates the delivery (HTTP basic auth, compared in constant
//     time; the hook must use HTTPS, which ADO requires for basic auth);
//   - ignores everything but a comment with a "/vizzini" line, on an active
//     pull request of an allowlisted repository, that the bot did not write;
//   - answers 202 at once and queues one job per pull request: deliveries
//     for a PR already waiting are folded into that job, and a delivery for
//     a PR whose job is running makes it run once more afterwards;
//   - for each job, fetches the PR into a cached clone and runs
//     `vizzini-bot --commands-only`, which answers the pending commands and
//     reviews nothing. Posting is still authorized only by the grant policy
//     on the base branch, and each command is still answered once.
//
// Only Azure DevOps is supported: mrman cannot reply in GitHub or GitLab
// threads yet.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxBody = 1 << 20

type config struct {
	listen        string
	tlsCert       string
	tlsKey        string
	user          string
	secret        string
	repos         map[string]bool
	work          string
	bot           string
	git           string
	grantPolicy   string
	maxCommands   int
	maxJobs       int
	jobTimeout    time.Duration
	botArgs       []string
	commandAuthor string
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func parseConfig(args []string, stderr io.Writer, getenv func(string) string) (*config, error) {
	c := &config{repos: map[string]bool{}}
	fs := flag.NewFlagSet("vizzini-webhook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var repos, botArgs stringList
	var secretFile string
	fs.StringVar(&c.listen, "listen", "127.0.0.1:8484", "address to listen on (put it behind an HTTPS reverse proxy, or set --tls-cert/--tls-key)")
	fs.StringVar(&c.tlsCert, "tls-cert", "", "serve HTTPS with this certificate (PEM)")
	fs.StringVar(&c.tlsKey, "tls-key", "", "the certificate's key (PEM)")
	fs.StringVar(&c.user, "user", "vizzini", "the basic-auth user name the service hook sends")
	fs.StringVar(&secretFile, "secret-file", getenv("VIZZINI_WEBHOOK_SECRET_FILE"), "file holding the basic-auth password (or set VIZZINI_WEBHOOK_SECRET)")
	fs.Var(&repos, "repo", `an allowed repository, "dev.azure.com/ORG/PROJECT/REPO" (repeatable; at least one)`)
	fs.StringVar(&c.work, "work", "", "directory for the cached clones and job data (required)")
	fs.StringVar(&c.bot, "bot", "vizzini-bot", "the vizzini-bot binary")
	fs.StringVar(&c.git, "git", "git", "the git binary")
	fs.StringVar(&c.grantPolicy, "grant-policy", ".vizzini/grant-policy", "passed to vizzini-bot: relative paths are read from the base branch")
	fs.IntVar(&c.maxCommands, "max-commands", 5, "passed to vizzini-bot: most commands answered per job")
	fs.StringVar(&c.commandAuthor, "command-authors", "", "passed to vizzini-bot: comma-separated users who may run commands (default anyone)")
	fs.IntVar(&c.maxJobs, "max-jobs", 2, "jobs run at once, across pull requests")
	fs.DurationVar(&c.jobTimeout, "job-timeout", 10*time.Minute, "time limit for one job")
	fs.Var(&botArgs, "bot-arg", "an extra vizzini-bot argument, e.g. --bot-arg=--knowledge=.okfw (repeatable)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	c.botArgs = botArgs
	for _, r := range repos {
		key, err := normalizeRepo(r)
		if err != nil {
			return nil, fmt.Errorf("--repo %q: %w", r, err)
		}
		c.repos[key] = true
	}
	c.secret = getenv("VIZZINI_WEBHOOK_SECRET")
	if secretFile != "" {
		b, err := os.ReadFile(secretFile) //nolint:gosec // G304: the operator names the secret file
		if err != nil {
			return nil, fmt.Errorf("--secret-file: %w", err)
		}
		c.secret = strings.TrimSpace(string(b))
	}
	switch {
	case len(c.repos) == 0:
		return nil, errors.New("at least one --repo is required")
	case c.work == "":
		return nil, errors.New("--work is required")
	case len(c.secret) < 16:
		return nil, errors.New("the basic-auth password (--secret-file or VIZZINI_WEBHOOK_SECRET) must be at least 16 characters")
	case (c.tlsCert == "") != (c.tlsKey == ""):
		return nil, errors.New("--tls-cert and --tls-key go together")
	case getenv("AZURE_DEVOPS_EXT_PAT") == "":
		return nil, errors.New("AZURE_DEVOPS_EXT_PAT is not set: the token the receiver fetches and replies with")
	case c.maxJobs < 1 || c.maxCommands < 1:
		return nil, errors.New("--max-jobs and --max-commands must be at least 1")
	}
	return c, nil
}

// server is the HTTP side: authenticate, filter, enqueue.
type server struct {
	cfg   *config
	queue *queue
	log   *slog.Logger
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/hooks/azure-devops", s.azureDevOps)
	return mux
}

func (s *server) authorized(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	// Both compared in constant time, always both.
	u := subtle.ConstantTimeCompare([]byte(user), []byte(s.cfg.user))
	p := subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.secret))
	return ok && u&p == 1
}

func reply(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body+"\n")
}

func (s *server) azureDevOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		reply(w, http.StatusMethodNotAllowed, `{"error":"POST only"}`)
		return
	}
	if !s.authorized(r) {
		s.log.Warn("delivery refused: bad credentials", "remote", r.RemoteAddr)
		w.Header().Set("WWW-Authenticate", `Basic realm="vizzini-webhook"`)
		reply(w, http.StatusUnauthorized, `{"error":"unauthorized"}`)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		reply(w, http.StatusRequestEntityTooLarge, `{"error":"body too large or unreadable"}`)
		return
	}
	ev, err := parseADOEvent(body)
	if err != nil {
		s.log.Warn("delivery refused: bad payload", "error", err)
		reply(w, http.StatusBadRequest, fmt.Sprintf(`{"error":%q}`, err.Error()))
		return
	}
	if reason := s.ignore(ev); reason != "" {
		s.log.Info("delivery ignored", "reason", reason, "event", ev.EventType, "pr", ev.key())
		reply(w, http.StatusOK, fmt.Sprintf(`{"accepted":false,"reason":%q}`, reason))
		return
	}
	state := s.queue.add(ev.job())
	s.log.Info("delivery accepted", "pr", ev.key(), "comment", ev.CommentID, "queue", state)
	reply(w, http.StatusAccepted, fmt.Sprintf(`{"accepted":true,"pr":%q,"queue":%q}`, ev.key(), state))
}

// ignore says why an authenticated event is not acted on ("" acts).
func (s *server) ignore(ev *adoEvent) string {
	switch {
	case ev.EventType != adoCommentEvent:
		return "not a pull request comment event"
	case !s.cfg.repos[ev.Repo]:
		return "repository not allowlisted"
	case ev.Status != "active":
		return "pull request not active"
	case ev.CommentType != "" && ev.CommentType != "text":
		return "not a text comment"
	case strings.Contains(ev.Content, "<!-- vizzini:"):
		return "written by the bot"
	case !hasCommand(ev.Content):
		return "no /vizzini command"
	}
	return ""
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr, os.Getenv))
}

func run(args []string, stderr io.Writer, getenv func(string) string) int {
	cfg, err := parseConfig(args, stderr, getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, "vizzini-webhook:", err)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	log.Warn("vizzini-webhook is EXPERIMENTAL: Azure DevOps only; answers /vizzini commands with vizzini-bot --commands-only")
	if merr := os.MkdirAll(cfg.work, 0o700); merr != nil {
		log.Error("work directory", "error", merr)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := &runner{cfg: cfg, log: log, command: execCommand}
	q := newQueue(cfg.maxJobs, r.run, log)
	s := &server{cfg: cfg, queue: q, log: log}
	srv := &http.Server{Addr: cfg.listen, Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.listen)
	if err != nil {
		log.Error("listen", "error", err)
		return 1
	}
	log.Info("listening", "addr", ln.Addr().String(), "tls", cfg.tlsCert != "", "repos", len(cfg.repos))
	var wg sync.WaitGroup
	var serveErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		if cfg.tlsCert != "" {
			serveErr = srv.ServeTLS(ln, cfg.tlsCert, cfg.tlsKey)
		} else {
			serveErr = srv.Serve(ln)
		}
	}()
	<-ctx.Done()
	log.Info("shutting down: no new deliveries; waiting for running jobs")
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.jobTimeout+30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	q.close(shutdown)
	wg.Wait()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Error("serve", "error", serveErr)
		return 1
	}
	return 0
}
