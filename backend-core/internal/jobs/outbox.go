package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
)

const (
	FieldPayload     = "payload"
	FieldTraceParent = "traceparent"
	FieldOutboxID    = "outbox_id"
	FieldError       = "error"
	FieldDeliveries  = "deliveries"
	FieldOriginalID  = "original_id"
	DLQSuffix        = ".dlq"
)

type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func Enqueue(ctx context.Context, q Querier, topic string, payload any) error {
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.enqueue")
	defer span.End()
	body, err := json.Marshal(payload)
	if err != nil {
		return apperr.From(fmt.Errorf("marshal outbox payload: %w", err))
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	_, err = q.Exec(ctx, `INSERT INTO outbox (topic, payload, trace_parent) VALUES ($1, $2, $3)`,
		topic, body, carrier.Get(FieldTraceParent))
	if err != nil {
		return apperr.From(fmt.Errorf("insert outbox: %w", err))
	}
	return nil
}
