package runstream

import "github.com/google/uuid"

const (
	// keyPrefix names one Redis Stream per run: run:{runID}.
	keyPrefix = "run:"
	// FieldEvent holds the JSON envelope of one model.Event; FieldRequestID the request id of the run (W9).
	FieldEvent     = "event"
	FieldRequestID = "request_id"
)

// StreamKey is the Redis Stream key of a run.
func StreamKey(runID uuid.UUID) string { return keyPrefix + runID.String() }
