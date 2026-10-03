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

type Relay struct {
	Pool         *pgxpool.Pool
	Redis        *redis.Client
	BatchSize    int
	PollInterval time.Duration
	Log          *slog.Logger
}

func (r *Relay) Run(ctx context.Context) error {
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.relay")
	defer span.End()
	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := r.PublishBatch(ctx); err != nil {
			r.Log.ErrorContext(ctx, "outbox relay batch failed", "error_code", errors.From(err).Code)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Relay) PublishBatch(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return 0, errors.From(fmt.Errorf("begin relay tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id, topic, payload, trace_parent FROM outbox
		WHERE published_at IS NULL ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, r.BatchSize)
	if err != nil {
		return 0, errors.From(fmt.Errorf("select outbox: %w", err))
	}
	type row struct{ id, topic, payload, traceParent string }
	var batch []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.id, &x.topic, &x.payload, &x.traceParent); err != nil {
			rows.Close()
			return 0, errors.From(fmt.Errorf("scan outbox: %w", err))
		}
		batch = append(batch, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, errors.From(fmt.Errorf("read outbox: %w", err))
	}

	for _, x := range batch {
		values := map[string]any{FieldPayload: x.payload, FieldTraceParent: x.traceParent, FieldOutboxID: x.id}
		if err := r.Redis.XAdd(ctx, &redis.XAddArgs{Stream: x.topic, Values: values}).Err(); err != nil {
			return 0, errors.From(fmt.Errorf("xadd %s: %w", x.topic, err))
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1`, x.id); err != nil {
			return 0, errors.From(fmt.Errorf("mark published: %w", err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, errors.From(fmt.Errorf("commit relay tx: %w", err))
	}
	return len(batch), nil
}
