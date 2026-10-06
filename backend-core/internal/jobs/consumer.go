package jobs

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	busyGroupPrefix = "BUSYGROUP"
	streamStartID   = "0"
	newMessagesID   = ">"
	autoClaimStart  = "0-0"
	fieldDurationMS = "duration_ms"
)

type Message struct {
	TenantID      string
	OutboxID      string
	StreamEntryID string
	Stream        string
	Payload       []byte
	RequestID     string
	Deliveries    int64
}

type Handler func(ctx context.Context, m Message) error

type Consumer struct {
	Redis             *redis.Client
	Stream            string
	GroupName         string
	ConsumerName      string
	MaxDeliveries     int
	VisibilityTimeout time.Duration
	BlockTimeout      time.Duration
	BatchSize         int64
	Log               *slog.Logger
}

func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.Redis.XGroupCreateMkStream(ctx, c.Stream, c.GroupName, streamStartID).Err()
	if err != nil && !strings.HasPrefix(err.Error(), busyGroupPrefix) {
		return errors.ToAppError(fmt.Errorf("create group %s/%s: %w", c.Stream, c.GroupName, err))
	}
	return nil
}

func (c *Consumer) Run(ctx context.Context, handler Handler) error {
	for ctx.Err() == nil {
		if err := c.PollOnce(ctx, handler); err != nil && !stderrors.Is(err, context.Canceled) {
			c.Log.ErrorContext(ctx, "stream poll failed", "stream", c.Stream, "error_code", errors.CodeOf(err))
		}
	}
	return nil
}

func (c *Consumer) PollOnce(ctx context.Context, handler Handler) error {
	reclaimed, _, err := c.Redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: c.Stream, Group: c.GroupName, Consumer: c.ConsumerName, MinIdle: c.VisibilityTimeout, Start: autoClaimStart, Count: c.BatchSize,
	}).Result()
	if err != nil {
		return errors.ToAppError(fmt.Errorf("xautoclaim: %w", err))
	}
	c.handleEntries(ctx, reclaimed, handler)

	streams, err := c.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: c.GroupName, Consumer: c.ConsumerName, Streams: []string{c.Stream, newMessagesID}, Count: c.BatchSize, Block: c.BlockTimeout,
	}).Result()
	if stderrors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return errors.ToAppError(fmt.Errorf("xreadgroup: %w", err))
	}
	for _, s := range streams {
		c.handleEntries(ctx, s.Messages, handler)
	}
	return nil
}

func (c *Consumer) handleEntries(ctx context.Context, msgs []redis.XMessage, handler Handler) {
	for _, raw := range msgs {
		c.handle(ctx, raw, handler)
	}
}

// handle restores the originating request id and tenant, so every log line of the job can be found from the request.
func (c *Consumer) handle(ctx context.Context, raw redis.XMessage, handler Handler) {
	tenantID, _ := raw.Values[StreamFieldTenantID].(string)
	requestID := messageRequestID(raw)
	ctx = logging.ContextWithRequestID(tenant.WithID(ctx, tenantID), requestID)
	deliveries, err := c.deliveries(ctx, raw.ID)
	if err != nil {
		c.Log.ErrorContext(ctx, "read delivery count failed", "id", raw.ID, "error_code", errors.CodeOf(err))
		return
	}
	msg := toMessage(c.Stream, raw, requestID, deliveries)
	start := time.Now()
	herr := handler(ctx, msg)
	elapsed := time.Since(start).Milliseconds()
	if herr == nil {
		c.ack(ctx, raw.ID)
		c.Log.InfoContext(ctx, "job handled", "stream", c.Stream, "id", raw.ID, "outbox_id", msg.OutboxID, "deliveries", deliveries, fieldDurationMS, elapsed)
		return
	}
	if deliveries >= int64(c.MaxDeliveries) {
		c.deadLetter(ctx, msg, herr, elapsed)
		return
	}
	c.Log.WarnContext(ctx, "job failed, will retry", "stream", c.Stream, "id", raw.ID, "outbox_id", msg.OutboxID, "deliveries", deliveries, "error_code", errors.CodeOf(herr), fieldDurationMS, elapsed)
}

// messageRequestID trusts only well-formed ids; entries from an older relay or forged entries get a fresh job id.
func messageRequestID(raw redis.XMessage) string {
	if id, _ := raw.Values[StreamFieldRequestID].(string); requestid.IsValid(id) {
		return id
	}
	return requestid.NewJob()
}

func (c *Consumer) deliveries(ctx context.Context, id string) (int64, error) {
	pending, err := c.Redis.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: c.Stream, Group: c.GroupName, Start: id, End: id, Count: 1}).Result()
	if err != nil {
		return 0, errors.ToAppError(fmt.Errorf("xpending: %w", err))
	}
	if len(pending) == 0 {
		return 1, nil
	}
	return pending[0].RetryCount, nil
}

func (c *Consumer) ack(ctx context.Context, id string) {
	if err := c.Redis.XAck(ctx, c.Stream, c.GroupName, id).Err(); err != nil {
		c.Log.ErrorContext(ctx, "xack failed", "id", id, "error_code", errors.CodeOf(err))
	}
}

func (c *Consumer) deadLetter(ctx context.Context, m Message, cause error, elapsed int64) {
	values := map[string]any{
		StreamFieldPayload: string(m.Payload), StreamFieldRequestID: m.RequestID, StreamFieldError: cause.Error(),
		StreamFieldDeliveries: m.Deliveries, StreamFieldOriginalID: m.StreamEntryID, StreamFieldOutboxID: m.OutboxID,
		StreamFieldTenantID: m.TenantID,
	}
	if err := c.Redis.XAdd(ctx, &redis.XAddArgs{Stream: c.Stream + DeadLetterSuffix, Values: values}).Err(); err != nil {
		c.Log.ErrorContext(ctx, "dead-letter failed; leaving message pending", "id", m.StreamEntryID, "error_code", errors.CodeOf(err))
		return
	}
	c.ack(ctx, m.StreamEntryID)
	c.Log.ErrorContext(ctx, "job moved to DLQ", "stream", c.Stream, "id", m.StreamEntryID, "outbox_id", m.OutboxID, "deliveries", m.Deliveries, "error_code", errors.CodeOf(cause), fieldDurationMS, elapsed)
}

func toMessage(stream string, raw redis.XMessage, requestID string, deliveries int64) Message {
	payload, _ := raw.Values[StreamFieldPayload].(string)
	outboxID, _ := raw.Values[StreamFieldOutboxID].(string)
	tenantID, _ := raw.Values[StreamFieldTenantID].(string)
	return Message{TenantID: tenantID, OutboxID: outboxID, StreamEntryID: raw.ID, Stream: stream, Payload: []byte(payload), RequestID: requestID, Deliveries: deliveries}
}
