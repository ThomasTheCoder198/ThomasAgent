package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	apperrors "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	platformjobs "github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/telemetry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const (
	workerWait       = 10 * time.Second
	workerTestStream = "thomas.worker.test"
	expiredToken     = "expired-token"
)

func startWorkerTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	container, err := tcredis.Run(t.Context(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	address, err := container.ConnectionString(t.Context())
	require.NoError(t, err)
	options, err := redis.ParseURL(address)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	return client
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

// find returns the records whose msg equals msg.
func (b *syncBuffer) find(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range b.records(t) {
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

type workerEnv struct {
	pool           *pgxpool.Pool
	redis          *redis.Client
	redisContainer *tcredis.RedisContainer
	databaseURL    string
	redisURL       string
}

func startWorkerEnv(t *testing.T) workerEnv {
	t.Helper()
	database, err := tcpostgres.Run(t.Context(), "postgres:18.6-alpine", tcpostgres.WithDatabase("core"), tcpostgres.WithUsername("core"), tcpostgres.WithPassword("core"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Terminate(context.Background()) })
	databaseURL, err := database.ConnectionString(t.Context(), "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, postgres.Migrate(t.Context(), databaseURL, migrations.FS, postgres.Up))
	pool, err := postgres.Open(t.Context(), databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	redisContainer, err := tcredis.Run(t.Context(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(context.Background()) })
	redisURL, err := redisContainer.ConnectionString(t.Context())
	require.NoError(t, err)
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	return workerEnv{pool: pool, redis: client, redisContainer: redisContainer, databaseURL: databaseURL, redisURL: redisURL}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}

func workerRuntime(t *testing.T, env workerEnv, addr string, drain time.Duration) (*coreRuntime, *syncBuffer) {
	t.Helper()
	out := &syncBuffer{}
	log, err := logging.NewLogger(out, "debug", "core-test", "test", nil)
	require.NoError(t, err)
	return &coreRuntime{
		cfg: config.Config{
			DatabaseURL: env.databaseURL, RedisURL: env.redisURL, ShutdownTimeout: time.Second, ReadHeaderTimeout: time.Second,
			Worker: config.WorkerConfig{Addr: addr, DrainTimeout: drain, ChatConcurrency: 2},
			Stream: config.ConsumerConfig{MaxDeliveries: 3, BlockTimeout: 50 * time.Millisecond, BatchSize: 1},
			Chat: config.ChatConfig{RunDeadline: time.Minute, FinishTimeout: 5 * time.Second, JobVisibilityTimeout: 2 * time.Minute, RunLease: 30 * time.Second,
				RunHeartbeatTimeout: 5 * time.Second, RunStreamMaxEvents: 100, RunStreamMaxBytes: 1 << 20, RunStreamGrace: time.Minute, RelayStatusCheck: time.Second},
			OutboxRelay:  config.OutboxRelayConfig{BatchSize: 10, PollInterval: 10 * time.Millisecond},
			Auth:         config.AuthConfig{SessionPurgeInterval: 20 * time.Millisecond},
			Conversation: config.ConversationConfig{RetentionPurgeInterval: time.Hour}, SessionPurgeTimeout: time.Minute, ConversationRetentionPurgeTimeout: time.Minute,
		},
		log: log, metrics: metrics.New([]float64{1}, []float64{1}),
		telemetry: telemetry.Telemetry{Shutdown: func(context.Context) error { return nil }},
	}, out
}

func httpGet(t *testing.T, url string) (int, string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		return 0, ""
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(body)
}

func waitHealthy(t *testing.T, addr string) {
	t.Helper()
	require.Eventually(t, func() bool { status, _ := httpGet(t, "http://"+addr+"/healthz"); return status == http.StatusOK }, workerWait, 20*time.Millisecond, "worker did not start serving")
}

type workerRun struct {
	cancel context.CancelFunc
	done   chan error
}

func startServeWorker(t *testing.T, rt *coreRuntime, env workerEnv, jobs []periodic.Job) workerRun {
	t.Helper()
	ctx, cancel := context.WithCancel(tenant.WithID(t.Context(), tenant.PlatformID))
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		done <- rt.serveWorker(ctx, env.pool, env.redis, jobs, completingExecutor{store: newTestChatStore(env.pool, rt)})
	}()
	return workerRun{cancel: cancel, done: done}
}

func (w workerRun) stop(t *testing.T) error {
	t.Helper()
	w.cancel()
	select {
	case err := <-w.done:
		return err
	case <-time.After(workerWait):
		t.Fatal("worker did not stop")
		return nil
	}
}

func idleJob(name string) periodic.Job {
	return periodic.Job{Name: name, Interval: time.Hour, Timeout: time.Minute, Run: func(context.Context) (int64, error) { return 0, nil }}
}

func TestWorker_ServesHealthReadyAndMetrics(t *testing.T) {
	env := startWorkerEnv(t)
	addr := freeAddr(t)
	rt, _ := workerRuntime(t, env, addr, time.Second)
	run := startServeWorker(t, rt, env, []periodic.Job{idleJob("idle")})
	waitHealthy(t, addr)
	status, _ := httpGet(t, "http://"+addr+"/readyz")
	require.Equal(t, http.StatusOK, status)
	require.Eventually(t, func() bool {
		status, body := httpGet(t, "http://"+addr+"/metrics")
		return status == http.StatusOK && strings.Contains(body, "outbox_pending")
	}, workerWait, 20*time.Millisecond, "the relay must publish its backlog gauge to /metrics")
	require.NoError(t, run.stop(t))
}

func TestWorker_ReadyzFailsWhenRedisIsDownButHealthzStaysUp(t *testing.T) {
	env := startWorkerEnv(t)
	addr := freeAddr(t)
	rt, _ := workerRuntime(t, env, addr, time.Second)
	run := startServeWorker(t, rt, env, []periodic.Job{idleJob("idle")})
	waitHealthy(t, addr)
	require.NoError(t, env.redisContainer.Terminate(context.Background()))
	require.Eventually(t, func() bool {
		status, _ := httpGet(t, "http://"+addr+"/readyz")
		return status == http.StatusServiceUnavailable
	}, workerWait, 50*time.Millisecond)
	status, _ := httpGet(t, "http://"+addr+"/healthz")
	require.Equal(t, http.StatusOK, status, "liveness must not depend on dependencies")
	require.NoError(t, run.stop(t))
}

func TestWorker_RelayPublishesOutboxRows(t *testing.T) {
	env := startWorkerEnv(t)
	rt, _ := workerRuntime(t, env, freeAddr(t), time.Second)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	id, err := platformjobs.EnqueueOutbox(ctx, env.pool, workerTestStream, map[string]string{"n": "1"})
	require.NoError(t, err)
	run := startServeWorker(t, rt, env, []periodic.Job{idleJob("idle")})
	require.Eventually(t, func() bool { return env.redis.XLen(ctx, workerTestStream).Val() == 1 }, workerWait, 20*time.Millisecond)
	entries, err := env.redis.XRange(ctx, workerTestStream, "-", "+").Result()
	require.NoError(t, err)
	require.Equal(t, id, entries[0].Values[platformjobs.StreamFieldOutboxID])
	require.NoError(t, run.stop(t))
}

// withClientConfig gives a runtime the settings that building the clients and the run loop needs (the vault key, the
// app signing key, a ragx address and the provider HTTP policy), as `core serve` and `core worker` both do.
func withClientConfig(t *testing.T, rt *coreRuntime) {
	t.Helper()
	rt.cfg.Vault = config.VaultConfig{KeyID: "v1", MasterKey: testVaultMasterKey}
	rt.cfg.App = config.AppConfig{SigningKey: testSigningKeyPEM(t), SigningKeyID: "test-1", TokenIssuer: "thomas-core", TokenAudience: "ragx", TokenTTL: time.Minute}
	rt.cfg.Ragx = config.RagxConfig{URL: "http://127.0.0.1:1", ConnectTimeout: time.Second, RequestTimeout: time.Second, ResponseHeaderTimeout: time.Second, MaxErrorBodyBytes: 1 << 10, MaxResponseBytes: 1 << 10, ProvisionAttempts: 1, ToolTimeout: time.Second}
	rt.cfg.ProviderHTTPTimeout = time.Second
	rt.cfg.ProviderMaxResponseBytes = config.DefaultProviderMaxResponseBytes
	rt.cfg.ProviderBreaker = config.BreakerConfig{FailureThreshold: 5, OpenTimeout: time.Second, HalfOpenMaxCalls: 1}
}

func TestWorker_RunsPeriodicJobsWithJobRequestID(t *testing.T) {
	env := startWorkerEnv(t)
	rt, out := workerRuntime(t, env, freeAddr(t), time.Second)
	withClientConfig(t, rt)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- rt.runWorker(ctx) }()
	require.Eventually(t, func() bool { return len(out.find(t, "periodic job completed")) > 0 }, workerWait, 20*time.Millisecond, "the session purge job must run in the worker")
	record := out.find(t, "periodic job completed")[0]
	require.Equal(t, sessionPurgeJob, record["job"])
	id, _ := record["request_id"].(string)
	require.True(t, strings.HasPrefix(id, requestid.JobPrefix))
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(workerWait):
		t.Fatal("worker did not stop")
	}
	require.Len(t, out.find(t, "worker stopped"), 1)
}

func blockingJob(entered chan<- struct{}, release <-chan struct{}, ctxAliveAfterRelease *atomic.Bool) periodic.Job {
	return periodic.Job{Name: "blocking", Interval: 10 * time.Millisecond, Timeout: time.Minute, Run: func(ctx context.Context) (int64, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		ctxAliveAfterRelease.Store(ctx.Err() == nil)
		return 1, nil
	}}
}

func TestWorker_DrainWaitsForInFlightJob(t *testing.T) {
	env := startWorkerEnv(t)
	addr := freeAddr(t)
	rt, out := workerRuntime(t, env, addr, workerWait)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var alive atomic.Bool
	run := startServeWorker(t, rt, env, []periodic.Job{blockingJob(entered, release, &alive)})
	select {
	case <-entered:
	case <-time.After(workerWait):
		t.Fatal("job did not start")
	}
	run.cancel()
	select {
	case <-run.done:
		t.Fatal("the worker stopped while a job was still running")
	case <-time.After(200 * time.Millisecond):
	}
	status, _ := httpGet(t, "http://"+addr+"/healthz")
	require.Equal(t, http.StatusOK, status, "probes stay up while draining")
	close(release)
	select {
	case err := <-run.done:
		require.NoError(t, err)
	case <-time.After(workerWait):
		t.Fatal("the worker did not stop after the job finished")
	}
	require.True(t, alive.Load(), "SIGTERM must not cancel the running job")
	require.Empty(t, out.find(t, "worker drain timed out"))
	require.Len(t, out.find(t, "worker stopped"), 1)
}

func TestWorker_DrainTimeoutWarnsAndStops(t *testing.T) {
	env := startWorkerEnv(t)
	rt, out := workerRuntime(t, env, freeAddr(t), 100*time.Millisecond)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var alive atomic.Bool
	run := startServeWorker(t, rt, env, []periodic.Job{blockingJob(entered, release, &alive)})
	<-entered
	require.NoError(t, run.stop(t))
	require.Len(t, out.find(t, "worker drain timed out"), 1)
	require.Len(t, out.find(t, "worker stopped"), 1)
	close(release)
}

func TestWorker_ListenerFailureStopsBackgroundLoops(t *testing.T) {
	env := startWorkerEnv(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = occupied.Close() }()
	rt, out := workerRuntime(t, env, occupied.Addr().String(), time.Second)
	var runs atomic.Int32
	job := periodic.Job{Name: "counting", Interval: 10 * time.Millisecond, Timeout: time.Minute, Run: func(context.Context) (int64, error) { runs.Add(1); return 0, nil }}
	run := startServeWorker(t, rt, env, []periodic.Job{job})
	select {
	case err := <-run.done:
		require.ErrorContains(t, err, "worker listener")
	case <-time.After(workerWait):
		t.Fatal("a listener failure must end the worker")
	}
	settled := runs.Load()
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, settled, runs.Load(), "periodic jobs must be stopped before the worker returns and its pool closes")
	require.Len(t, out.find(t, "worker stopped"), 1)
}

func TestOutboxRelayAlias_LogsOneDeprecationLineAndRunsWorker(t *testing.T) {
	for command, wantDeprecations := range map[string]int{outboxRelayCommand: 1, workerCommand: 0} {
		t.Run(command, func(t *testing.T) {
			env := startWorkerEnv(t)
			rt, out := workerRuntime(t, env, freeAddr(t), time.Second)
			withClientConfig(t, rt)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- executeCommandWith(ctx, command, nil, func(context.Context) (*coreRuntime, error) { return rt, nil })
			}()
			require.Eventually(t, func() bool { return len(out.find(t, "worker started")) == 1 }, workerWait, 20*time.Millisecond)
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(workerWait):
				t.Fatal("worker did not stop")
			}
			require.Len(t, out.find(t, "outbox-relay command is deprecated; use worker"), wantDeprecations)
		})
	}
}

func TestServeHTTP_DoesNotRunPeriodicJobs(t *testing.T) {
	env := startWorkerEnv(t)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	_, err := env.pool.Exec(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ($1, 'purge@example.com', 'x')`, tenant.PlatformID)
	require.NoError(t, err)
	_, err = env.pool.Exec(ctx, `INSERT INTO sessions (tenant_id, user_id, token_hash, csrf_token, expires_at)
		SELECT $1, id, convert_to($2, 'UTF8'), 'csrf', now() - interval '1 hour' FROM users LIMIT 1`, tenant.PlatformID, expiredToken)
	require.NoError(t, err)
	rt, out := workerRuntime(t, env, "", time.Second)
	rt.cfg.HTTPAddr = freeAddr(t)
	withClientConfig(t, rt)
	rt.cfg.Auth = config.AuthConfig{OwnerEmail: "owner@example.com", OwnerPassword: "correct horse battery", SessionTTL: time.Hour, LoginMaxAttempts: 10, LoginWindow: time.Minute, MinPasswordLength: 12, SessionPurgeInterval: 10 * time.Millisecond}
	rt.cfg.HTTP = config.HTTPConfig{MaxBodyBytes: 1 << 10, RequestTimeout: time.Second}
	serveCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- rt.serveHTTP(serveCtx) }()
	require.Eventually(t, func() bool { return len(out.find(t, coreListeningMessage)) == 1 }, workerWait, 20*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	var remaining int
	require.NoError(t, env.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash = convert_to($1, 'UTF8')`, expiredToken).Scan(&remaining))
	require.Equal(t, 1, remaining, "the API must leave expired-session cleanup to the worker")
	require.Empty(t, out.find(t, "periodic job completed"))
}

func TestHealthcheck_WorkerTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/healthz", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	setIdentityEnv(t)
	t.Setenv("CORE_DATABASE_URL", "postgres://invalid:invalid@127.0.0.1:1/none")
	t.Setenv("CORE_REDIS_URL", "redis://127.0.0.1:1")
	t.Setenv("CORE_LOGS_OTLP_ENDPOINT", "")
	t.Setenv("CORE_WORKER_ADDR", strings.TrimPrefix(server.URL, "http://127.0.0.1"))
	require.NoError(t, executeCommand(t.Context(), "healthcheck", []string{"--target", "worker"}))
}

func TestWorker_InvalidPeriodicJobEndsTheWorkerWithAnError(t *testing.T) {
	env := startWorkerEnv(t)
	rt, out := workerRuntime(t, env, freeAddr(t), time.Second)
	broken := idleJob("broken")
	broken.Interval = 0
	run := startServeWorker(t, rt, env, []periodic.Job{broken})
	select {
	case err := <-run.done:
		require.ErrorIs(t, err, apperrors.ErrPeriodicJobInvalid)
	case <-time.After(workerWait):
		t.Fatal("a worker that cannot run its jobs must not idle silently")
	}
	require.Len(t, out.find(t, "worker stopped"), 1)
}

func TestHealthcheck_RejectsUnknownTarget(t *testing.T) {
	setIdentityEnv(t)
	t.Setenv("CORE_DATABASE_URL", "postgres://invalid:invalid@127.0.0.1:1/none")
	t.Setenv("CORE_REDIS_URL", "redis://127.0.0.1:1")
	t.Setenv("CORE_LOGS_OTLP_ENDPOINT", "")
	for _, args := range [][]string{{"--target", "bogus"}, {"--target"}, {"bogus", "api"}} {
		require.ErrorIs(t, executeCommand(t.Context(), "healthcheck", args), apperrors.ErrHealthcheckTargetInvalid, args)
	}
}
