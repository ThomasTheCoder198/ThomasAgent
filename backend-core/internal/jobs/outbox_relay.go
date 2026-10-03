package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type OutboxRelay struct {
	Pool         *pgxpool.Pool
	Redis        *redis.Client
	BatchSize    int
	PollInterval time.Duration
	Log          *slog.Logger
}

func (r *OutboxRelay) Run(ctx context.Context) error {
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, outboxRelaySpan)
	defer span.End()
	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := r.PublishBatch(ctx); err != nil {
			r.Log.ErrorContext(ctx, "outbox relay batch failed", "error_code", errors.CodeOf(err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *OutboxRelay) PublishBatch(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, publishOutboxSpan, trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return 0, errors.ToAppError(fmt.Errorf("begin relay tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id, topic, payload, trace_parent FROM outbox
		WHERE published_at IS NULL ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, r.BatchSize)
	if err != nil {
		return 0, errors.ToAppError(fmt.Errorf("select outbox: %w", err))
	}
	type row struct{ id, stream, payload, traceParent string }
	var batch []row
	for rows.Next() {
		var entry row
		if err := rows.Scan(&entry.id, &entry.stream, &entry.payload, &entry.traceParent); err != nil {
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
		values := map[string]any{StreamFieldPayload: entry.payload, StreamFieldTraceParent: entry.traceParent, StreamFieldOutboxID: entry.id}
		if err := r.Redis.XAdd(ctx, &redis.XAddArgs{Stream: entry.stream, Values: values}).Err(); err != nil {
			return 0, errors.ToAppError(fmt.Errorf("xadd %s: %w", entry.stream, err))
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1`, entry.id); err != nil {
			return 0, errors.ToAppError(fmt.Errorf("mark published: %w", err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, errors.ToAppError(fmt.Errorf("commit relay tx: %w", err))
	}
	return len(batch), nil
}
