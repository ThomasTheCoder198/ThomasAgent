package periodic

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	lockPrefix   = "lock:job:"
	streamPrefix = "periodic:"
	logKeyJob    = "job"
	// releaseTimeout bounds the unlock so a slow Redis cannot hold shutdown; the lock expires on its own anyway.
	releaseTimeout = 5 * time.Second
)

var releaseLock = redis.NewScript(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end`)

type Job struct {
	Name     string
	Interval time.Duration
	Timeout  time.Duration
	Run      func(context.Context) (int64, error)
}

type JobRecorder interface{ ObserveJob(string, string) }

type Runner struct {
	Redis   *redis.Client
	Log     *slog.Logger
	Metrics JobRecorder
}

// Run ticks every job until ctx is cancelled, then waits for the runs already in flight. A job that is running
// when shutdown starts keeps its own context and deadline, so a purge is never cut off half way. The first run of
// a job happens one Interval after Run starts.
func (r Runner) Run(ctx context.Context, jobs []Job) error {
	for _, job := range jobs {
		if err := job.validate(); err != nil {
			return err
		}
	}
	var loops sync.WaitGroup
	for _, job := range jobs {
		loops.Add(1)
		go func() {
			defer loops.Done()
			ticker := time.NewTicker(job.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// A tick can win the select against a cancelled context.
					if ctx.Err() != nil {
						return
					}
					r.runOne(ctx, job)
				}
			}
		}()
	}
	loops.Wait()
	return nil
}

func (j Job) validate() error {
	if j.Name == "" || j.Run == nil || j.Interval <= 0 || j.Timeout <= 0 {
		return fmt.Errorf("%w: job %q", errors.ErrPeriodicJobInvalid, j.Name)
	}
	return nil
}

func (r Runner) observe(job Job, outcome string) {
	if r.Metrics != nil {
		r.Metrics.ObserveJob(streamPrefix+job.Name, outcome)
	}
}

func (r Runner) runOne(ctx context.Context, job Job) {
	key, token := lockPrefix+job.Name, uuid.NewString()
	locked, err := r.Redis.SetNX(ctx, key, token, job.Timeout).Result()
	if err != nil {
		r.observe(job, metrics.OutcomeError)
		r.Log.ErrorContext(ctx, "periodic lock failed", logKeyJob, job.Name, logging.FieldError, err)
		return
	}
	if !locked {
		r.Log.DebugContext(ctx, "periodic lock busy", logKeyJob, job.Name)
		return
	}
	// Shutdown stops new runs but not this one; the timeout still bounds it.
	jobCtx, cancel := context.WithTimeout(requestid.NewContext(context.WithoutCancel(ctx), requestid.NewJob()), job.Timeout)
	start := time.Now()
	count, runErr := runRecovering(jobCtx, job)
	cancel()
	r.release(ctx, job, key, token)
	outcome := metrics.OutcomeSuccess
	if runErr != nil {
		outcome = metrics.OutcomeError
	}
	r.observe(job, outcome)
	if runErr != nil {
		r.Log.ErrorContext(jobCtx, "periodic job failed", logKeyJob, job.Name, logging.FieldCount, count, logging.FieldDurationMS, time.Since(start).Milliseconds(), logging.FieldError, runErr)
		return
	}
	r.Log.InfoContext(jobCtx, "periodic job completed", logKeyJob, job.Name, logging.FieldCount, count, logging.FieldDurationMS, time.Since(start).Milliseconds())
}

// release deletes the lock only if this run still owns it: a timed-out lock can already belong to another worker.
func (r Runner) release(ctx context.Context, job Job, key, token string) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if err := releaseLock.Run(releaseCtx, r.Redis, []string{key}, token).Err(); err != nil {
		r.Log.WarnContext(releaseCtx, "periodic lock release failed", logKeyJob, job.Name, logging.FieldError, err)
	}
}

// runRecovering turns a panic into an error, so one bad job neither kills the worker nor keeps its lock until expiry.
func runRecovering(ctx context.Context, job Job) (count int64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("periodic job panicked: %v", recovered)
		}
	}()
	return job.Run(ctx)
}
