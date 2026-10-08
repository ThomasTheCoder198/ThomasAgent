package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type BacklogRecorder interface {
	SetOutboxBacklog(pending int, oldest time.Duration)
}

type OutboxRelay struct {
	Pool         *pgxpool.Pool
	Redis        *redis.Client
	BatchSize    int
	PollInterval time.Duration
	Log          *slog.Logger
	Metrics      BacklogRecorder
}

func (r *OutboxRelay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := r.PublishBatch(ctx); err != nil {
			r.Log.ErrorContext(ctx, "outbox relay batch failed", "error_code", errors.CodeOf(err))
		}
		r.recordBacklog(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// The relay is a system worker, so backlog includes every tenant like the publish query.
func (r *OutboxRelay) recordBacklog(ctx context.Context) {
	if r.Metrics == nil {
		return
	}
	var pending int
	var oldestSeconds float64
	err := r.Pool.QueryRow(ctx, `SELECT count(*), COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0)::float8
		FROM outbox WHERE published_at IS NULL`).Scan(&pending, &oldestSeconds)
	if err != nil {
		r.Log.WarnContext(ctx, "outbox backlog query failed", "error_code", errors.CodeOf(errors.ToAppError(err)))
		return
	}
	r.Metrics.SetOutboxBacklog(pending, time.Duration(oldestSeconds*float64(time.Second)))
}

func (r *OutboxRelay) PublishBatch(ctx context.Context) (int, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return 0, errors.ToAppError(fmt.Errorf("begin relay tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id, tenant_id, topic, payload, request_id FROM outbox
		WHERE published_at IS NULL ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, r.BatchSize)
	if err != nil {
		return 0, errors.ToAppError(fmt.Errorf("select outbox: %w", err))
	}
	type row struct{ id, tenantID, stream, payload, requestID string }
	var batch []row
	for rows.Next() {
		var entry row
		if err := rows.Scan(&entry.id, &entry.tenantID, &entry.stream, &entry.payload, &entry.requestID); err != nil {
			rows.Close()
			return 0, errors.ToAppError(fmt.Errorf("scan outbox: %w", err))
		}
		batch = append(batch, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, errors.ToAppError(fmt.Errorf("read outbox: %w", err))
	}

	for _, entry := range batch {
		values := map[string]any{StreamFieldPayload: entry.payload, StreamFieldRequestID: entry.requestID, StreamFieldOutboxID: entry.id, StreamFieldTenantID: entry.tenantID}
		if err := r.Redis.XAdd(ctx, &redis.XAddArgs{Stream: entry.stream, Values: values}).Err(); err != nil {
			return 0, errors.ToAppError(fmt.Errorf("xadd %s: %w", entry.stream, err))
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1 AND tenant_id=$2`, entry.id, entry.tenantID); err != nil {
			return 0, errors.ToAppError(fmt.Errorf("mark published: %w", err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, errors.ToAppError(fmt.Errorf("commit relay tx: %w", err))
	}
	return len(batch), nil
}
