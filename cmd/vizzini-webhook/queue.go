package main

import (
	"context"
	"log/slog"
	"sync"
)

// job is one vizzini-bot --commands-only run for one pull request.
type job struct {
	Key       string // "ado:dev.azure.com/ORG/PROJECT/REPO#ID"
	Repo      string // "dev.azure.com/ORG/PROJECT/REPO"
	CloneURL  string
	PRID      int
	Target    string
	WebURL    string
	Requester string
}

// Queue states, as add reports them.
const (
	stateQueued    = "queued"    // a new job
	stateCoalesced = "coalesced" // folded into the PR's job that is waiting
	stateAgain     = "again"     // the PR's job is running; it runs once more after
	stateFull      = "full"      // too many pull requests waiting
	stateClosed    = "closed"    // shutting down
)

const queueDepth = 256

// queue runs at most one job per pull request at a time and folds
// deliveries together: a burst of comments on one PR costs one run, and a
// comment that arrives while its PR's job runs (which may already have read
// the threads) gets one more run.
type queue struct {
	mu      sync.Mutex
	jobs    map[string]*entry
	ready   chan string
	closed  bool
	workers sync.WaitGroup
	run     func(context.Context, *job)
	log     *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
}

type entry struct {
	job     *job
	running bool
	again   bool
}

func newQueue(workers int, run func(context.Context, *job), log *slog.Logger) *queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &queue{jobs: map[string]*entry{}, ready: make(chan string, queueDepth), run: run, log: log, ctx: ctx, cancel: cancel}
	for range workers {
		q.workers.Add(1)
		go q.work()
	}
	return q
}

func (q *queue) add(j *job) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return stateClosed
	}
	if e, ok := q.jobs[j.Key]; ok {
		e.job = j
		if e.running {
			e.again = true
			return stateAgain
		}
		return stateCoalesced
	}
	select {
	case q.ready <- j.Key:
		q.jobs[j.Key] = &entry{job: j}
		return stateQueued
	default:
		return stateFull
	}
}

func (q *queue) work() {
	defer q.workers.Done()
	for key := range q.ready {
		q.mu.Lock()
		e := q.jobs[key]
		e.running = true
		j := e.job
		q.mu.Unlock()

		q.run(q.ctx, j)

		q.mu.Lock()
		if e.again && !q.closed {
			e.running, e.again = false, false
			select {
			case q.ready <- key:
			default:
				delete(q.jobs, key)
				q.log.Warn("queue full: follow-up run dropped", "pr", key)
			}
		} else {
			delete(q.jobs, key)
		}
		q.mu.Unlock()
	}
}

// close stops taking jobs, lets the running ones finish until ctx ends
// (then cancels them), and drops the ones still waiting.
func (q *queue) close(ctx context.Context) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	// Drain what waits; running jobs are not in the channel.
	for {
		select {
		case key := <-q.ready:
			delete(q.jobs, key)
			q.log.Info("waiting job dropped at shutdown", "pr", key)
			continue
		default:
		}
		break
	}
	close(q.ready)
	q.mu.Unlock()
	done := make(chan struct{})
	go func() { q.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		q.cancel()
		<-done
	}
	q.cancel()
}
