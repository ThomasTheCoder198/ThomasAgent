package main

import (
	"context"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	sessionPurgeFailedMessage = "expired session purge failed"
	sessionsPurgedMessage     = "expired sessions purged"
)

type sessionPurger interface {
	PurgeExpiredSessionsAcrossTenants(context.Context) (int64, error)
}

func (rt *coreRuntime) purgeExpiredSessions(ctx context.Context, service sessionPurger) {
	if rt.cfg.Auth.SessionPurgeInterval <= 0 {
		return
	}
	ticker := time.NewTicker(rt.cfg.Auth.SessionPurgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.runSessionPurge(ctx, service)
		}
	}
}

// Each scheduled run gets a job id so its log lines can be found together.
func (rt *coreRuntime) runSessionPurge(ctx context.Context, service sessionPurger) {
	ctx = requestid.NewContext(ctx, requestid.NewJob())
	start := time.Now()
	count, err := service.PurgeExpiredSessionsAcrossTenants(ctx)
	if err != nil {
		rt.log.ErrorContext(ctx, sessionPurgeFailedMessage, logging.FieldError, err, logging.FieldDurationMS, time.Since(start).Milliseconds())
		return
	}
	rt.log.InfoContext(ctx, sessionsPurgedMessage, logging.FieldCount, count, logging.FieldDurationMS, time.Since(start).Milliseconds())
}
