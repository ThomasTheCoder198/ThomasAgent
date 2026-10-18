package main

import (
	"context"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	conversationPurgeFailedMessage = "inactive conversation purge failed"
	conversationsPurgedMessage     = "inactive conversations purged"
	conversationPurgeJob           = "conversation_retention"
)

type conversationPurger interface {
	PurgeInactiveConversations(context.Context) (int64, error)
}

func (rt *coreRuntime) purgeInactiveConversations(ctx context.Context, service conversationPurger) {
	if rt.cfg.Conversation.RetentionPurgeInterval <= 0 {
		return
	}
	ticker := time.NewTicker(rt.cfg.Conversation.RetentionPurgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.runConversationPurge(ctx, service)
		}
	}
}

func (rt *coreRuntime) runConversationPurge(ctx context.Context, service conversationPurger) {
	ctx = requestid.NewContext(ctx, requestid.NewJob())
	start := time.Now()
	count, err := service.PurgeInactiveConversations(ctx)
	if err != nil {
		if rt.metrics != nil {
			rt.metrics.ObserveJob(conversationPurgeJob, metrics.OutcomeError)
		}
		rt.log.ErrorContext(ctx, conversationPurgeFailedMessage, logging.FieldError, err, logging.FieldCount, count, logging.FieldDurationMS, time.Since(start).Milliseconds())
		return
	}
	if rt.metrics != nil {
		rt.metrics.ObserveJob(conversationPurgeJob, metrics.OutcomeSuccess)
	}
	rt.log.InfoContext(ctx, conversationsPurgedMessage, logging.FieldCount, count, logging.FieldDurationMS, time.Since(start).Milliseconds())
}
