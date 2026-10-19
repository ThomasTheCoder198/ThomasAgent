package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
)

const conversationPurgeJob = "conversation_retention"

type conversationPurger interface {
	PurgeInactiveConversations(context.Context) (int64, error)
}

func (rt *coreRuntime) conversationPurgeJob(service conversationPurger) periodic.Job {
	return periodic.Job{Name: conversationPurgeJob, Interval: rt.cfg.Conversation.RetentionPurgeInterval, Timeout: rt.cfg.ConversationRetentionPurgeTimeout, Run: service.PurgeInactiveConversations}
}
