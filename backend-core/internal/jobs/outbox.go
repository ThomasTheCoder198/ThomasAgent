package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	StreamFieldPayload    = "payload"
	StreamFieldRequestID  = "request_id"
	StreamFieldTenantID   = "tenant_id"
	StreamFieldOutboxID   = "outbox_id"
	StreamFieldError      = "error"
	StreamFieldDeliveries = "deliveries"
	StreamFieldOriginalID = "original_id"
	DeadLetterSuffix      = ".dlq"
)

type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// EnqueueOutbox stores the request id with the job, so the job's logs can be found from the request that caused it.
func EnqueueOutbox(ctx context.Context, q Execer, stream string, payload any) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.ToAppError(fmt.Errorf("marshal outbox payload: %w", err))
	}
	_, err = q.Exec(ctx, `INSERT INTO outbox (tenant_id, topic, payload, request_id) VALUES ($1, $2, $3, $4)`,
		tenantID, stream, body, originRequestID(ctx))
	if err != nil {
		return errors.ToAppError(fmt.Errorf("insert outbox: %w", err))
	}
	return nil
}

// originRequestID gives work that does not start from a request (a schedule, a startup task) its own job id.
func originRequestID(ctx context.Context) string {
	if id := logging.RequestIDFromContext(ctx); requestid.IsValid(id) {
		return id
	}
	return requestid.NewJob()
}
