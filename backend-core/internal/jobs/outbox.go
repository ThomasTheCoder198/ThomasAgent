package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	StreamFieldPayload     = "payload"
	StreamFieldTraceParent = "traceparent"
	StreamFieldOutboxID    = "outbox_id"
	StreamFieldError       = "error"
	StreamFieldDeliveries  = "deliveries"
	StreamFieldOriginalID  = "original_id"
	DeadLetterSuffix       = ".dlq"
)

type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func EnqueueOutbox(ctx context.Context, q Execer, stream string, payload any) error {
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, enqueueOutboxSpan)
	defer span.End()
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.ToAppError(fmt.Errorf("marshal outbox payload: %w", err))
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	_, err = q.Exec(ctx, `INSERT INTO outbox (topic, payload, trace_parent) VALUES ($1, $2, $3)`,
		stream, body, carrier.Get(StreamFieldTraceParent))
	if err != nil {
		return errors.ToAppError(fmt.Errorf("insert outbox: %w", err))
	}
	return nil
}
