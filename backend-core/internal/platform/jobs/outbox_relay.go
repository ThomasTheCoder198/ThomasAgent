package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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
	for ctx.Err() == nil {
		// A batch that has started runs to its end: cancelling between XADD and COMMIT would publish the row twice.
		batchCtx := context.WithoutCancel(ctx)
		if _, err := r.PublishBatch(batchCtx); err != nil {
			r.Log.ErrorContext(batchCtx, "outbox relay batch failed", "error_code", errors.CodeOf(err))
		}
		r.recordBacklog(batchCtx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
	return nil
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
	return r.publish(ctx, "")
}

func (r *OutboxRelay) PublishOne(ctx context.Context, id string) error {
	_, err := r.publish(ctx, id)
	return err
}

func (r *OutboxRelay) publish(ctx context.Context, onlyID string) (int, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return 0, errors.ToAppError(fmt.Errorf("begin relay tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	batch, err := r.selectBatch(ctx, tx, onlyID)
	if err != nil {
		return 0, err
	}
	for _, entry := range batch {
		values := map[string]any{StreamFieldPayload: entry.payload, StreamFieldRequestID: entry.requestID, StreamFieldOutboxID: entry.id, StreamFieldTenantID: entry.tenantID, StreamFieldTraceParent: entry.traceParent}
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

type outboxRow struct{ id, tenantID, stream, payload, requestID, traceParent string }

func (r *OutboxRelay) selectBatch(ctx context.Context, tx pgx.Tx, onlyID string) ([]outboxRow, error) {
	query := `SELECT id, tenant_id, topic, payload, request_id, COALESCE(traceparent, '') FROM outbox
		WHERE published_at IS NULL ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`
	args := []any{r.BatchSize}
	if onlyID != "" {
		query = `SELECT id, tenant_id, topic, payload, request_id, COALESCE(traceparent, '') FROM outbox
			WHERE id=$1 AND published_at IS NULL FOR UPDATE SKIP LOCKED`
		args = []any{onlyID}
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.ToAppError(fmt.Errorf("select outbox: %w", err))
	}
	var batch []outboxRow
	for rows.Next() {
		var entry outboxRow
		if err := rows.Scan(&entry.id, &entry.tenantID, &entry.stream, &entry.payload, &entry.requestID, &entry.traceParent); err != nil {
			rows.Close()
			return nil, errors.ToAppError(fmt.Errorf("scan outbox: %w", err))
		}
		batch = append(batch, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, errors.ToAppError(fmt.Errorf("read outbox: %w", err))
	}
	return batch, nil
}
