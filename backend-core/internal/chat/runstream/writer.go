package runstream

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	commandExpireAt = "EXPIREAT"
	// optionNX sets the expiry only when the key has none yet (Redis 7).
	optionNX = "NX"
)

// Writer appends a run's events to its Redis Stream (amendment W4). It has no MAXLEN: nothing is trimmed during a
// run, and the worker caps the run instead. Every entry carries the run's request id.
type Writer struct {
	redis *redis.Client
	// finishTimeout and grace extend the stream's life past the run deadline, so a late reader can still find the terminal event.
	finishTimeout time.Duration
	grace         time.Duration
}

func NewWriter(client *redis.Client, finishTimeout, grace time.Duration) *Writer {
	return &Writer{redis: client, finishTimeout: finishTimeout, grace: grace}
}

// Append writes one event. The expiry (deadline + finish timeout + grace) is set with NX in the same MULTI as the
// XADD, so the first write sets it and later writes leave it alone, with no state kept between calls or processes.
func (w *Writer) Append(ctx context.Context, runID uuid.UUID, deadline time.Time, event model.Event) error {
	raw, err := EncodeEvent(event)
	if err != nil {
		return err
	}
	key := StreamKey(runID)
	_, err = w.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.XAdd(ctx, &redis.XAddArgs{Stream: key, Values: map[string]any{FieldEvent: string(raw), FieldRequestID: requestid.FromContext(ctx)}})
		pipe.Do(ctx, commandExpireAt, key, deadline.Add(w.finishTimeout).Add(w.grace).Unix(), optionNX)
		return nil
	})
	if err != nil {
		return errors.ToAppError(fmt.Errorf("append run event: %w", err))
	}
	return nil
}

// WriteTerminal writes the event that ends the stream: finish or error, after FinishRun committed.
func (w *Writer) WriteTerminal(ctx context.Context, runID uuid.UUID, deadline time.Time, event model.Event) error {
	if !IsTerminal(event) {
		return fmt.Errorf("%w: %T is not a terminal event", errors.ErrRunEventInvalid, event)
	}
	return w.Append(ctx, runID, deadline, event)
}
