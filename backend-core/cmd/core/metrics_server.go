package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

const metricsServerStoppedMessage = "metrics server stopped"

// serveMetrics runs the internal /metrics listener until ctx ends. A failure is logged, never fatal: losing metrics
// must not take the API down.
func (rt *coreRuntime) serveMetrics(ctx context.Context) {
	if rt.cfg.Metrics.Addr == "" || rt.metrics == nil {
		return
	}
	srv := &http.Server{
		Addr:              rt.cfg.Metrics.Addr,
		Handler:           rt.metrics.Handler(),
		ReadHeaderTimeout: rt.cfg.ReadHeaderTimeout,
	}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), rt.cfg.ShutdownTimeout)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		case <-stopped:
		}
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		rt.log.ErrorContext(ctx, metricsServerStoppedMessage, logging.FieldError, err)
	}
}
