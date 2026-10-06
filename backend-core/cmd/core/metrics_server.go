package main

import (
	"context"
	"errors"
	"net/http"
)

// serveMetrics runs the internal /metrics listener until ctx ends. A failure is logged, never fatal: losing metrics
// must not take the API down.
func (a *application) serveMetrics(ctx context.Context) {
	if a.cfg.Metrics.Addr == "" || a.metrics == nil {
		return
	}
	srv := &http.Server{Addr: a.cfg.Metrics.Addr, Handler: a.metrics.Handler(), ReadHeaderTimeout: a.cfg.ReadHeaderTimeout}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		case <-stopped:
		}
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.log.ErrorContext(ctx, "metrics server stopped", "error", err)
	}
}
