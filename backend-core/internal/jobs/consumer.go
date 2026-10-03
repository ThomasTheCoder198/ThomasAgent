package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
)

const (
	busyGroupPrefix = "BUSYGROUP"
	streamStartID   = "0"
	newMessagesID   = ">"
	autoClaimStart  = "0-0"
)

type Message struct {
	OutboxID    string
	ID          string
	Stream      string
	Payload     []byte
	TraceParent string
	Deliveries  int64
}

type Handler func(ctx context.Context, m Message) error

type Consumer struct {
	Redis             *redis.Client
	Stream            string
	Group             string
	Name              string
	MaxDeliveries     int
	VisibilityTimeout time.Duration
	BlockTimeout      time.Duration
	BatchSize         int64
	Log               *slog.Logger
}

func (c *Consumer) EnsureGroup(ctx context.Context) error {
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.ensure-group", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	err := c.Redis.XGroupCreateMkStream(ctx, c.Stream, c.Group, streamStartID).Err()
	if err != nil && !strings.HasPrefix(err.Error(), busyGroupPrefix) {
		return apperr.From(fmt.Errorf("create group %s/%s: %w", c.Stream, c.Group, err))
	}
	return nil
}

func (c *Consumer) Run(ctx context.Context, h Handler) error {
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.consume")
	defer span.End()
	for ctx.Err() == nil {
		if err := c.Poll(ctx, h); err != nil && !errors.Is(err, context.Canceled) {
			c.Log.ErrorContext(ctx, "stream poll failed", "stream", c.Stream, "error_code", apperr.From(err).Code)
		}
	}
	return nil
}

func (c *Consumer) Poll(ctx context.Context, h Handler) error {
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.poll", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	reclaimed, _, err := c.Redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: c.Stream, Group: c.Group, Consumer: c.Name, MinIdle: c.VisibilityTimeout, Start: autoClaimStart, Count: c.BatchSize,
	}).Result()
	if err != nil {
		return apperr.From(fmt.Errorf("xautoclaim: %w", err))
	}
	c.handleAll(ctx, reclaimed, h)

	streams, err := c.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: c.Group, Consumer: c.Name, Streams: []string{c.Stream, newMessagesID}, Count: c.BatchSize, Block: c.BlockTimeout,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return apperr.From(fmt.Errorf("xreadgroup: %w", err))
	}
	for _, s := range streams {
		c.handleAll(ctx, s.Messages, h)
	}
	return nil
}

func (c *Consumer) handleAll(ctx context.Context, msgs []redis.XMessage, h Handler) {
	for _, raw := range msgs {
		c.handle(ctx, raw, h)
	}
}

func (c *Consumer) handle(ctx context.Context, raw redis.XMessage, h Handler) {
	parent, _ := raw.Values[FieldTraceParent].(string)
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{FieldTraceParent: parent})
	ctx, span := otel.Tracer("jobs").Start(ctx, "jobs.handle", trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()
	deliveries, err := c.deliveries(ctx, raw.ID)
	if err != nil {
		span.SetStatus(codes.Error, "delivery count failed")
		c.Log.ErrorContext(ctx, "read delivery count failed", "id", raw.ID, "error_code", apperr.From(err).Code)
		return
	}
	msg := toMessage(c.Stream, raw, deliveries)
	c.Log.DebugContext(ctx, "job handling", "stream", c.Stream, "id", raw.ID, "deliveries", deliveries)
	herr := h(ctx, msg)
	if herr == nil {
		c.ack(ctx, raw.ID)
		return
	}
	span.SetStatus(codes.Error, "job failed")
	if deliveries >= int64(c.MaxDeliveries) {
		c.deadLetter(ctx, msg, herr)
		return
	}
	c.Log.WarnContext(ctx, "job failed, will retry", "stream", c.Stream, "id", raw.ID, "deliveries", deliveries, "error_code", apperr.From(herr).Code)

}

func (c *Consumer) deliveries(ctx context.Context, id string) (int64, error) {
	pending, err := c.Redis.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: c.Stream, Group: c.Group, Start: id, End: id, Count: 1}).Result()
	if err != nil {
		return 0, apperr.From(fmt.Errorf("xpending: %w", err))
	}
	if len(pending) == 0 {
		return 1, nil
	}
	return pending[0].RetryCount, nil
}

func (c *Consumer) ack(ctx context.Context, id string) {
	if err := c.Redis.XAck(ctx, c.Stream, c.Group, id).Err(); err != nil {
		c.Log.ErrorContext(ctx, "xack failed", "id", id, "error_code", apperr.From(err).Code)
	}
}

func (c *Consumer) deadLetter(ctx context.Context, m Message, cause error) {
	values := map[string]any{
		FieldPayload: string(m.Payload), FieldTraceParent: m.TraceParent, FieldError: cause.Error(),
		FieldDeliveries: m.Deliveries, FieldOriginalID: m.ID, FieldOutboxID: m.OutboxID,
	}
	if err := c.Redis.XAdd(ctx, &redis.XAddArgs{Stream: c.Stream + DLQSuffix, Values: values}).Err(); err != nil {
		c.Log.ErrorContext(ctx, "dead-letter failed; leaving message pending", "id", m.ID, "error_code", apperr.From(err).Code)
		return
	}
	c.ack(ctx, m.ID)
	c.Log.ErrorContext(ctx, "job moved to DLQ", "stream", c.Stream, "id", m.ID, "deliveries", m.Deliveries, "error_code", apperr.From(cause).Code)
}

func toMessage(stream string, raw redis.XMessage, deliveries int64) Message {
	payload, _ := raw.Values[FieldPayload].(string)
	traceParent, _ := raw.Values[FieldTraceParent].(string)
	outboxID, _ := raw.Values[FieldOutboxID].(string)
	return Message{OutboxID: outboxID, ID: raw.ID, Stream: stream, Payload: []byte(payload), TraceParent: traceParent, Deliveries: deliveries}
}
