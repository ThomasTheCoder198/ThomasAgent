package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/control"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	chatworker "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	platformjobs "github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

const (
	// workerLoopCount is the number of background loops serveWorker starts next to the probe server: the outbox
	// relay, the periodic jobs and the chat.runs consumer.
	workerLoopCount = 3
	// chatRunsGroup is the consumer group every worker replica joins on chat.runs.
	chatRunsGroup = "chat-run-workers"
	// chatConsumerDependency is the readiness check that the chat.runs group exists.
	chatConsumerDependency = "chat_runs_group"
	// chatBatchSize is one entry per poll: a poller runs its batch serially, and a run lasts minutes.
	chatBatchSize = 1
	// ownerSuffixLength shortens a uuid into a per-process suffix for the lease owner and consumer name.
	ownerSuffixLength = 8
)

// workerOwner names this process in lease_owner, logs and as its consumer name; it is unique per process.
func workerOwner() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker"
	}
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString()[:ownerSuffixLength])
}

// chatRunConsumer wires the chat.runs consumer to the run handler; the executor is the run loop (chat/service).
func (rt *coreRuntime) chatRunConsumer(pool *pgxpool.Pool, redisClient *redis.Client, executor chatworker.Executor) (*platformjobs.Consumer, *chatworker.Handler) {
	owner := workerOwner()
	store := convservice.NewChatStore(pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(), convrepository.NewRunRepo(), agentrepository.NewAgentRepo(), rt.cfg.Chat.FinishTimeout)
	handler := &chatworker.Handler{
		Store: store, Control: control.NewBus(redisClient),
		Stream:   runstream.NewWriter(redisClient, rt.cfg.Chat.FinishTimeout, rt.cfg.Chat.RunStreamGrace),
		Executor: executor,
		Cfg:      rt.cfg.Chat, Log: rt.log, Owner: owner,
	}
	consumer := &platformjobs.Consumer{
		Redis: redisClient, Stream: convmodel.StreamChatRuns, GroupName: chatRunsGroup, ConsumerName: owner,
		MaxDeliveries: rt.cfg.Stream.MaxDeliveries, VisibilityTimeout: rt.cfg.Chat.JobVisibilityTimeout, BlockTimeout: rt.cfg.Stream.BlockTimeout,
		BatchSize: chatBatchSize, Concurrency: rt.cfg.Worker.ChatConcurrency, DrainTimeout: rt.cfg.Worker.DrainTimeout, Log: rt.log,
	}
	if rt.metrics != nil { // a nil *Metrics inside the interface would panic on the first call
		handler.Metrics = rt.metrics
		consumer.Metrics = rt.metrics
	}
	return consumer, handler
}

func (rt *coreRuntime) workerJobs(pool *pgxpool.Pool) []periodic.Job {
	auth := authservice.NewAuthService(authrepository.NewUserRepo(pool), authrepository.NewSessionRepo(pool, rt.cfg.Auth.SessionTouchInterval), pool, rt.cfg.Auth, time.Now)
	retention := convservice.NewRetentionService(pool, convrepository.NewConversationRepo(), rt.cfg.Conversation, time.Now)
	return []periodic.Job{rt.sessionPurgeJob(auth), rt.conversationPurgeJob(retention)}
}

func (rt *coreRuntime) runWorker(ctx context.Context) error {
	ctx = tenant.WithID(ctx, tenant.PlatformID)
	cipher, err := vault.NewCipher(rt.cfg.Vault.KeyID, rt.cfg.Vault.MasterKey)
	if err != nil {
		return fmt.Errorf("CORE_VAULT_MASTER_KEY: %w", err)
	}
	signingKey, err := apptoken.LoadSigningKey(rt.cfg.App, os.ReadFile)
	if err != nil {
		return fmt.Errorf("CORE_APP_SIGNING_KEY: %w", err)
	}
	pool, err := rt.openPostgres(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	redisClient, err := rt.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()
	runService, err := rt.initWorkerRunService(pool, cipher, signingKey)
	if err != nil {
		return err
	}
	return rt.serveWorker(ctx, pool, redisClient, rt.workerJobs(pool), runService)
}

// serveWorker runs the HTTP probe server, the outbox relay, the chat.runs consumer and the periodic jobs until ctx
// ends or the listener fails. Either way the background loops are stopped and drained before it returns, because the
// caller closes the database pool right after.
func (rt *coreRuntime) serveWorker(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, jobs []periodic.Job, executor chatworker.Executor) error {
	chatConsumer, chatHandler := rt.chatRunConsumer(pool, redisClient, executor)
	// The group exists before the probe server answers, so readiness means "can read chat jobs".
	if err := chatConsumer.EnsureGroup(ctx); err != nil {
		return err
	}
	relay := &platformjobs.OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: rt.cfg.OutboxRelay.BatchSize, PollInterval: rt.cfg.OutboxRelay.PollInterval, Log: rt.log, Metrics: rt.metrics}
	runner := periodic.Runner{Redis: redisClient, Log: rt.log, Metrics: rt.metrics}
	server := &http.Server{Addr: rt.cfg.Worker.Addr, Handler: rt.workerHandler(pool, redisClient), ReadHeaderTimeout: rt.cfg.ReadHeaderTimeout}
	loopCtx, stopLoops := context.WithCancel(ctx)
	defer stopLoops()
	listenResult := make(chan error, 1)
	loopFailed := make(chan error, workerLoopCount)
	go func() { listenResult <- server.ListenAndServe() }()
	var loops sync.WaitGroup
	loops.Add(workerLoopCount)
	go func() { defer loops.Done(); reportLoopFailure(loopFailed, relay.Run(loopCtx)) }()
	go func() { defer loops.Done(); reportLoopFailure(loopFailed, runner.Run(loopCtx, jobs)) }()
	go func() {
		defer loops.Done()
		reportLoopFailure(loopFailed, chatConsumer.Run(loopCtx, chatHandler.Handle))
	}()
	rt.log.InfoContext(ctx, "worker started", logging.FieldAddr, rt.cfg.Worker.Addr)
	var runErr error
	select {
	case err := <-listenResult:
		if !stderrors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("worker listener: %w", err)
		}
	case err := <-loopFailed:
		runErr = fmt.Errorf("worker background loop: %w", err)
	case <-ctx.Done():
	}
	stopLoops()
	runErr = stderrors.Join(runErr, rt.stopWorker(ctx, server, &loops))
	rt.log.InfoContext(ctx, "worker stopped")
	return runErr
}

func reportLoopFailure(failed chan<- error, err error) {
	if err != nil {
		failed <- err
	}
}

// stopWorker waits for the loops up to the drain timeout, then stops the probe server. Probes stay up while draining.
func (rt *coreRuntime) stopWorker(ctx context.Context, server *http.Server, loops *sync.WaitGroup) error {
	drained := make(chan struct{})
	go func() { loops.Wait(); close(drained) }()
	timer := time.NewTimer(rt.cfg.Worker.DrainTimeout)
	defer timer.Stop()
	select {
	case <-drained:
	case <-timer.C:
		rt.log.WarnContext(ctx, "worker drain timed out", "drain_timeout_ms", rt.cfg.Worker.DrainTimeout.Milliseconds())
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rt.cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown worker HTTP: %w", err)
	}
	return nil
}

func (rt *coreRuntime) workerHandler(pool *pgxpool.Pool, redisClient *redis.Client) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(rt.log), httpx.LogAccess(rt.log), httpx.RecordMetrics(rt.metrics))
	checks := rt.readinessChecks(pool, redisClient)
	checks[chatConsumerDependency] = redisx.GroupProbe{Client: redisClient, Stream: convmodel.StreamChatRuns, Group: chatRunsGroup}
	httpx.MountHealth(router, checks)
	router.GET("/metrics", gin.WrapH(rt.metrics.Handler()))
	return router
}
