package periodic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	apperrors "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const waitFor = 5 * time.Second

type recorder struct {
	mu       sync.Mutex
	outcomes []string
}

func (r *recorder) ObserveJob(stream, outcome string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes = append(r.outcomes, stream+"/"+outcome)
}

func (r *recorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.outcomes...)
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	container, err := tcredis.Run(t.Context(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	url, err := container.ConnectionString(t.Context())
	require.NoError(t, err)
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func testRunner(t *testing.T) Runner {
	t.Helper()
	return Runner{Redis: testRedis(t), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func jsonLogger(t *testing.T) (*slog.Logger, *syncBuffer) {
	t.Helper()
	out := &syncBuffer{}
	log, err := logging.NewLogger(out, "debug", "core-test", "test", nil)
	require.NoError(t, err)
	return log, out
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) records(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		out = append(out, rec)
	}
	return out
}

func (b *syncBuffer) byLevel(t *testing.T, level string) []map[string]any {
	var out []map[string]any
	for _, rec := range b.records(t) {
		if rec["level"] == level {
			out = append(out, rec)
		}
	}
	return out
}

func noopJob(name string) Job {
	return Job{Name: name, Interval: time.Second, Timeout: time.Second, Run: func(context.Context) (int64, error) { return 0, nil }}
}

func TestRunnerRejectsInvalidJobs(t *testing.T) {
	cases := map[string]func(*Job){
		"zero interval":     func(j *Job) { j.Interval = 0 },
		"negative interval": func(j *Job) { j.Interval = -time.Second },
		"zero timeout":      func(j *Job) { j.Timeout = 0 },
		"negative timeout":  func(j *Job) { j.Timeout = -time.Second },
		"no run function":   func(j *Job) { j.Run = nil },
		"empty name":        func(j *Job) { j.Name = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			job := noopJob("guarded")
			mutate(&job)
			err := Runner{}.Run(t.Context(), []Job{job})
			require.ErrorIs(t, err, apperrors.ErrPeriodicJobInvalid)
		})
	}
}

func TestRunnerReturnsOnCancelledContextWithoutRunningJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var ran atomic.Int32
	job := noopJob("never")
	job.Interval = time.Nanosecond
	job.Run = func(context.Context) (int64, error) { ran.Add(1); return 0, nil }
	// A zero Runner has no Redis client; any attempt to run the job would panic.
	require.NoError(t, Runner{}.Run(ctx, []Job{job}))
	require.Zero(t, ran.Load())
}

func TestRunnerTicksAndRunsJob(t *testing.T) {
	runner := testRunner(t)
	var ran atomic.Int32
	job := noopJob("ticking")
	job.Interval = 10 * time.Millisecond
	job.Run = func(context.Context) (int64, error) { ran.Add(1); return 1, nil }
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, []Job{job}) }()
	require.Eventually(t, func() bool { return ran.Load() >= 2 }, waitFor, 5*time.Millisecond, "the ticker must run the job repeatedly")
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(waitFor):
		t.Fatal("runner did not stop")
	}
}

func TestRunnerTwoRunnersOneTickRunOnce(t *testing.T) {
	first := testRunner(t)
	second := Runner{Redis: first.Redis, Log: first.Log}
	var ran atomic.Int32
	release := make(chan struct{})
	job := noopJob("shared")
	job.Interval = 10 * time.Millisecond
	job.Timeout = time.Minute
	job.Run = func(context.Context) (int64, error) { ran.Add(1); <-release; return 0, nil }
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	for _, r := range []Runner{first, second} {
		wg.Add(1)
		go func() { defer wg.Done(); _ = r.Run(ctx, []Job{job}) }()
	}
	require.Eventually(t, func() bool { return ran.Load() == 1 }, waitFor, 5*time.Millisecond)
	// Both runners tick several times while the first run holds the lock.
	time.Sleep(100 * time.Millisecond)
	require.EqualValues(t, 1, ran.Load(), "the lock must let only one runner execute the job")
	cancel()
	close(release)
	wg.Wait()
}

func TestRunnerDrainsInFlightJobOnCancel(t *testing.T) {
	runner := testRunner(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var ctxErrAfterRelease atomic.Value
	job := noopJob("drain")
	job.Interval = 10 * time.Millisecond
	job.Run = func(ctx context.Context) (int64, error) {
		entered <- struct{}{}
		<-release
		ctxErrAfterRelease.Store(ctx.Err() == nil)
		return 1, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, []Job{job}) }()
	select {
	case <-entered:
	case <-time.After(waitFor):
		t.Fatal("job did not start")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while the job was still in flight")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(waitFor):
		t.Fatal("Run did not return after the job finished")
	}
	require.Equal(t, true, ctxErrAfterRelease.Load(), "shutdown must not cancel the running job's context")
	require.Zero(t, runner.Redis.Exists(context.Background(), lockPrefix+job.Name).Val())
}

func TestRunnerJobStillHonorsItsTimeoutAfterShutdown(t *testing.T) {
	runner := testRunner(t)
	entered := make(chan struct{}, 1)
	job := noopJob("bounded")
	job.Interval = 10 * time.Millisecond
	job.Timeout = 100 * time.Millisecond
	job.Run = func(ctx context.Context) (int64, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return 0, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, []Job{job}) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(waitFor):
		t.Fatal("a job past its timeout must not block shutdown forever")
	}
}

func TestRunnerFailureLogsOnceAndReleasesLock(t *testing.T) {
	runner := testRunner(t)
	log, out := jsonLogger(t)
	rec := &recorder{}
	runner.Log, runner.Metrics = log, rec
	job := noopJob("failing")
	job.Run = func(context.Context) (int64, error) { return 7, errors.New("expected failure") }
	runner.runOne(t.Context(), job)
	errs := out.byLevel(t, "ERROR")
	require.Len(t, errs, 1, "a failed job logs its error exactly once")
	require.Equal(t, job.Name, errs[0]["job"])
	require.EqualValues(t, 7, errs[0][logging.FieldCount])
	require.Contains(t, errs[0], logging.FieldDurationMS)
	require.Zero(t, runner.Redis.Exists(t.Context(), lockPrefix+job.Name).Val())
	require.Equal(t, []string{streamPrefix + job.Name + "/" + metrics.OutcomeError}, rec.seen())
}

func TestRunnerSuccessLogsCountJobIDAndDuration(t *testing.T) {
	runner := testRunner(t)
	log, out := jsonLogger(t)
	runner.Log = log
	job := noopJob("counting")
	job.Run = func(context.Context) (int64, error) { return 3, nil }
	runner.runOne(t.Context(), job)
	infos := out.byLevel(t, "INFO")
	require.Len(t, infos, 1)
	require.Equal(t, job.Name, infos[0]["job"])
	require.EqualValues(t, 3, infos[0][logging.FieldCount])
	require.Contains(t, infos[0], logging.FieldDurationMS)
	id, _ := infos[0]["request_id"].(string)
	require.True(t, strings.HasPrefix(id, requestid.JobPrefix), "each run gets its own job id, got %q", id)
	require.Empty(t, out.byLevel(t, "ERROR"))
}

func TestRunnerPanicIsRecoveredLoggedOnceAndReleasesLock(t *testing.T) {
	runner := testRunner(t)
	log, out := jsonLogger(t)
	rec := &recorder{}
	runner.Log, runner.Metrics = log, rec
	job := noopJob("panicking")
	job.Run = func(context.Context) (int64, error) { panic("boom") }
	require.NotPanics(t, func() { runner.runOne(t.Context(), job) })
	require.Len(t, out.byLevel(t, "ERROR"), 1)
	require.Zero(t, runner.Redis.Exists(t.Context(), lockPrefix+job.Name).Val())
	require.Equal(t, []string{streamPrefix + job.Name + "/" + metrics.OutcomeError}, rec.seen())
}

func TestRunnerLockFailureCarriesJobFieldAndMetric(t *testing.T) {
	client := testRedis(t)
	log, out := jsonLogger(t)
	rec := &recorder{}
	runner := Runner{Redis: client, Log: log, Metrics: rec}
	require.NoError(t, client.Close())
	var ran atomic.Int32
	job := noopJob("nolock")
	job.Run = func(context.Context) (int64, error) { ran.Add(1); return 0, nil }
	runner.runOne(t.Context(), job)
	require.Zero(t, ran.Load())
	errs := out.byLevel(t, "ERROR")
	require.Len(t, errs, 1)
	require.Equal(t, job.Name, errs[0]["job"])
	require.Equal(t, []string{streamPrefix + job.Name + "/" + metrics.OutcomeError}, rec.seen())
}

func TestRunnerLockAllowsOneRunAndReleasesAfterFailure(t *testing.T) {
	runner := testRunner(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var count atomic.Int32
	job := Job{Name: "lock-test", Interval: time.Millisecond, Timeout: time.Minute, Run: func(context.Context) (int64, error) {
		count.Add(1)
		entered <- struct{}{}
		<-release
		return 0, context.DeadlineExceeded
	}}
	go runner.runOne(t.Context(), job)
	select {
	case <-entered:
	case <-time.After(waitFor):
		t.Fatal("first job did not start")
	}
	runner.runOne(t.Context(), job)
	require.EqualValues(t, 1, count.Load())
	close(release)
	require.Eventually(t, func() bool { return runner.Redis.Exists(t.Context(), lockPrefix+job.Name).Val() == 0 }, waitFor, time.Millisecond)
	runner.runOne(t.Context(), job)
	require.EqualValues(t, 2, count.Load())
}

func TestRunnerExpiredLockDoesNotReleaseSecondRunsLock(t *testing.T) {
	runner := testRunner(t)
	firstEntered, secondEntered := make(chan struct{}), make(chan struct{})
	releaseFirst, releaseSecond := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	job := Job{Name: "expiry-test", Interval: time.Millisecond, Timeout: 50 * time.Millisecond, Run: func(context.Context) (int64, error) {
		if calls.Add(1) == 1 {
			close(firstEntered)
			<-releaseFirst
			return 0, nil
		}
		close(secondEntered)
		<-releaseSecond
		return 0, nil
	}}
	firstDone := make(chan struct{})
	go func() { runner.runOne(t.Context(), job); close(firstDone) }()
	<-firstEntered
	require.Eventually(t, func() bool { return runner.Redis.Exists(t.Context(), lockPrefix+job.Name).Val() == 0 }, waitFor, 5*time.Millisecond, "the lock must expire")
	go runner.runOne(t.Context(), job)
	select {
	case <-secondEntered:
	case <-time.After(waitFor):
		t.Fatal("second job did not start after expiry")
	}
	close(releaseFirst)
	<-firstDone
	require.EqualValues(t, 1, runner.Redis.Exists(t.Context(), lockPrefix+job.Name).Val(), "the first run must not delete the second run's lock")
	close(releaseSecond)
}
