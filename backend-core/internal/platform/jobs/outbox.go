package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/propagation"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	StreamFieldPayload     = "payload"
	StreamFieldRequestID   = "request_id"
	StreamFieldTenantID    = "tenant_id"
	StreamFieldOutboxID    = "outbox_id"
	StreamFieldError       = "error"
	StreamFieldDeliveries  = "deliveries"
	StreamFieldOriginalID  = "original_id"
	StreamFieldTraceParent = "traceparent"
	DeadLetterSuffix       = ".dlq"
	// traceParentHeader is the W3C Trace Context header name, not the stream field that carries its value.
	traceParentHeader = "traceparent"
)

type Execer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// EnqueueOutbox stores the request id with the job, so the job's logs can be found from the request that caused it.
func EnqueueOutbox(ctx context.Context, q Execer, stream string, payload any) (string, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", errors.ToAppError(fmt.Errorf("marshal outbox payload: %w", err))
	}
	// The W3C propagator is used directly: the global one is a no-op until something installs it.
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	var traceParent any
	if value := carrier.Get(traceParentHeader); value != "" {
		traceParent = value
	}
	var id string
	err = q.QueryRow(ctx, `INSERT INTO outbox (tenant_id, topic, payload, request_id, traceparent) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		tenantID, stream, body, originRequestID(ctx), traceParent).Scan(&id)
	if err != nil {
		return "", errors.ToAppError(fmt.Errorf("insert outbox: %w", err))
	}
	return id, nil
}

// originRequestID gives work that does not start from a request (a schedule, a startup task) its own job id.
func originRequestID(ctx context.Context) string {
	if id := requestid.FromContext(ctx); requestid.IsValid(id) {
		return id
	}
	return requestid.NewJob()
}
