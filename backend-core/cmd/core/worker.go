package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	platformjobs "github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// workerLoopCount is the number of background loops serveWorker starts next to the probe server.
const workerLoopCount = 2

func (rt *coreRuntime) workerJobs(pool *pgxpool.Pool) []periodic.Job {
	auth := authservice.NewAuthService(authrepository.NewUserRepo(pool), authrepository.NewSessionRepo(pool, rt.cfg.Auth.SessionTouchInterval), pool, rt.cfg.Auth, time.Now)
	retention := convservice.NewRetentionService(pool, convrepository.NewConversationRepo(), rt.cfg.Conversation, time.Now)
	return []periodic.Job{rt.sessionPurgeJob(auth), rt.conversationPurgeJob(retention)}
}

func (rt *coreRuntime) runWorker(ctx context.Context) error {
	ctx = tenant.WithID(ctx, tenant.PlatformID)
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
	return rt.serveWorker(ctx, pool, redisClient, rt.workerJobs(pool))
}

// serveWorker runs the HTTP probe server, the outbox relay and the periodic jobs until ctx ends or the listener
// fails. Either way the background loops are stopped and drained before it returns, because the caller closes the
// database pool right after.
func (rt *coreRuntime) serveWorker(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, jobs []periodic.Job) error {
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
	httpx.MountHealth(router, rt.readinessChecks(pool, redisClient))
	router.GET("/metrics", gin.WrapH(rt.metrics.Handler()))
	return router
}
