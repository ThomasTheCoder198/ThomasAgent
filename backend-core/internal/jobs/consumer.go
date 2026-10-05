package jobs

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	busyGroupPrefix = "BUSYGROUP"
	streamStartID   = "0"
	newMessagesID   = ">"
	autoClaimStart  = "0-0"
)

type Message struct {
	TenantID      string
	OutboxID      string
	StreamEntryID string
	Stream        string
	Payload       []byte
	TraceParent   string
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
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, ensureGroupSpan, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	err := c.Redis.XGroupCreateMkStream(ctx, c.Stream, c.GroupName, streamStartID).Err()
	if err != nil && !strings.HasPrefix(err.Error(), busyGroupPrefix) {
		return errors.ToAppError(fmt.Errorf("create group %s/%s: %w", c.Stream, c.GroupName, err))
	}
	return nil
}

func (c *Consumer) Run(ctx context.Context, handler Handler) error {
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, consumeSpan)
	defer span.End()
	for ctx.Err() == nil {
		if err := c.PollOnce(ctx, handler); err != nil && !stderrors.Is(err, context.Canceled) {
			c.Log.ErrorContext(ctx, "stream poll failed", "stream", c.Stream, "error_code", errors.CodeOf(err))
		}
	}
	return nil
}

func (c *Consumer) PollOnce(ctx context.Context, handler Handler) error {
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, pollSpan, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
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

func (c *Consumer) handle(ctx context.Context, raw redis.XMessage, handler Handler) {
	parent, _ := raw.Values[StreamFieldTraceParent].(string)
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{StreamFieldTraceParent: parent})
	tenantID, _ := raw.Values[StreamFieldTenantID].(string)
	ctx = tenant.WithID(ctx, tenantID)
	ctx, span := otel.Tracer(jobsTracerName).Start(ctx, handleSpan, trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()
	deliveries, err := c.deliveries(ctx, raw.ID)
	if err != nil {
		span.SetStatus(codes.Error, "delivery count failed")
		c.Log.ErrorContext(ctx, "read delivery count failed", "id", raw.ID, "error_code", errors.CodeOf(err))
		return
	}
	msg := toMessage(c.Stream, raw, deliveries)
	c.Log.DebugContext(ctx, "job handling", "stream", c.Stream, "id", raw.ID, "deliveries", deliveries)
	herr := handler(ctx, msg)
	if herr == nil {
		c.ack(ctx, raw.ID)
		return
	}
	span.SetStatus(codes.Error, "job failed")
	if deliveries >= int64(c.MaxDeliveries) {
		c.deadLetter(ctx, msg, herr)
		return
	}
	c.Log.WarnContext(ctx, "job failed, will retry", "stream", c.Stream, "id", raw.ID, "deliveries", deliveries, "error_code", errors.CodeOf(herr))

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

func (c *Consumer) deadLetter(ctx context.Context, m Message, cause error) {
	values := map[string]any{
		StreamFieldPayload: string(m.Payload), StreamFieldTraceParent: m.TraceParent, StreamFieldError: cause.Error(),
		StreamFieldDeliveries: m.Deliveries, StreamFieldOriginalID: m.StreamEntryID, StreamFieldOutboxID: m.OutboxID,
		StreamFieldTenantID: m.TenantID,
	}
	if err := c.Redis.XAdd(ctx, &redis.XAddArgs{Stream: c.Stream + DeadLetterSuffix, Values: values}).Err(); err != nil {
		c.Log.ErrorContext(ctx, "dead-letter failed; leaving message pending", "id", m.StreamEntryID, "error_code", errors.CodeOf(err))
		return
	}
	c.ack(ctx, m.StreamEntryID)
	c.Log.ErrorContext(ctx, "job moved to DLQ", "stream", c.Stream, "id", m.StreamEntryID, "deliveries", m.Deliveries, "error_code", errors.CodeOf(cause))
}

func toMessage(stream string, raw redis.XMessage, deliveries int64) Message {
	payload, _ := raw.Values[StreamFieldPayload].(string)
	traceParent, _ := raw.Values[StreamFieldTraceParent].(string)
	outboxID, _ := raw.Values[StreamFieldOutboxID].(string)
	tenantID, _ := raw.Values[StreamFieldTenantID].(string)
	return Message{TenantID: tenantID, OutboxID: outboxID, StreamEntryID: raw.ID, Stream: stream, Payload: []byte(payload), TraceParent: traceParent, Deliveries: deliveries}
}
