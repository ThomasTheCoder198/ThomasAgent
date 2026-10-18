package service

import (
	"context"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

type RetentionService interface {
	PurgeInactiveConversations(context.Context) (int64, error)
}

type RetentionServiceImpl struct {
	db   postgres.DBTX
	repo repository.ConversationRepo
	cfg  config.ConversationConfig
	now  func() time.Time
}

func NewRetentionService(db postgres.DBTX, repo repository.ConversationRepo, cfg config.ConversationConfig, now func() time.Time) RetentionService {
	return &RetentionServiceImpl{db: db, repo: repo, cfg: cfg, now: now}
}

func (s *RetentionServiceImpl) PurgeInactiveConversations(ctx context.Context) (int64, error) {
	cutoff := s.now().Add(-s.cfg.Retention)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, err := s.repo.PurgeInactiveAcrossTenants(ctx, s.db, cutoff, s.cfg.RetentionPurgeBatch)
		if err != nil {
			return total, err
		}
		total += count
		if count < int64(s.cfg.RetentionPurgeBatch) {
			return total, nil
		}
	}
}
