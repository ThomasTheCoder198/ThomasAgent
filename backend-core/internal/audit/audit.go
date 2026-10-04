package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

type Event struct {
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

func Record(ctx context.Context, db postgres.DBTX, e Event) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	traceID := ""
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	_, err = db.Exec(ctx, `INSERT INTO audit_events (actor, action, target_type, target_id, metadata, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6)`, e.Actor, e.Action, e.TargetType, e.TargetID, meta, traceID)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}
