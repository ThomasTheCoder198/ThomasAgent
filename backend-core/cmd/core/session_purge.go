package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
)

const sessionPurgeJob = "session_purge"

type sessionPurger interface {
	PurgeExpiredSessionsAcrossTenants(context.Context) (int64, error)
}

func (rt *coreRuntime) sessionPurgeJob(service sessionPurger) periodic.Job {
	return periodic.Job{Name: sessionPurgeJob, Interval: rt.cfg.Auth.SessionPurgeInterval, Timeout: rt.cfg.SessionPurgeTimeout, Run: service.PurgeExpiredSessionsAcrossTenants}
}
