package jobs

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
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
	RequestID     string
	TraceParent   string
	Deliveries    int64
}

type Handler func(ctx context.Context, m Message) error

type JobRecorder interface {
	ObserveJob(stream, outcome string)
}

type JobStreamRegistrar interface {
	RegisterJobStream(stream string)
}

type Consumer struct {
	Redis             *redis.Client
	Stream            string
	GroupName         string
	ConsumerName      string
	MaxDeliveries     int
	VisibilityTimeout time.Duration
	BlockTimeout      time.Duration
	BatchSize         int64
	Concurrency       int
	DrainTimeout      time.Duration
	Log               *slog.Logger
	Metrics           JobRecorder

	inFlight inFlightSet
}

// inFlightSet holds the entry ids this process is running. Pollers share one consumer name, so without it a
// sibling poller would reclaim an entry that is still running here and execute it a second time.
type inFlightSet struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

// claim marks the ids not yet running and returns only those.
func (s *inFlightSet) claim(ids []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = make(map[string]struct{})
	}
	var claimed []string
	for _, id := range ids {
		if _, running := s.ids[id]; !running {
			s.ids[id] = struct{}{}
			claimed = append(claimed, id)
		}
	}
	return claimed
}

func (s *inFlightSet) release(ids ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.ids, id)
	}
}

func (s *inFlightSet) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ids)
}

func (c *Consumer) observe(outcome string) {
	if c.Metrics != nil {
		c.Metrics.ObserveJob(c.Stream, outcome)
	}
}

func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.Redis.XGroupCreateMkStream(ctx, c.Stream, c.GroupName, streamStartID).Err()
	if err != nil && !strings.HasPrefix(err.Error(), busyGroupPrefix) {
		return errors.ToAppError(fmt.Errorf("create group %s/%s: %w", c.Stream, c.GroupName, err))
	}
	if registrar, ok := c.Metrics.(JobStreamRegistrar); ok {
		registrar.RegisterJobStream(c.Stream)
	}
	return nil
}

func (c *Consumer) Run(ctx context.Context, handler Handler) error {
	capacity := c.Concurrency
	if capacity < 1 {
		capacity = 1
	}
	var inFlight sync.WaitGroup
	for range capacity {
		inFlight.Add(1)
		go func() {
			defer inFlight.Done()
			for ctx.Err() == nil {
				if err := c.PollOnce(ctx, handler); err != nil && !stderrors.Is(err, context.Canceled) {
					c.Log.ErrorContext(ctx, "stream poll failed", "stream", c.Stream, "error_code", errors.CodeOf(err))
				}
			}
		}()
	}
	<-ctx.Done()
	finished := make(chan struct{})
	go func() { inFlight.Wait(); close(finished) }()
	if c.DrainTimeout <= 0 {
		<-finished
		return nil
	}
	timer := time.NewTimer(c.DrainTimeout)
	defer timer.Stop()
	select {
	case <-finished:
	case <-timer.C:
		// The handlers keep running detached; the caller must not close its pool as if they had stopped.
		c.Log.WarnContext(ctx, "consumer drain timed out with handlers still running", "stream", c.Stream, "in_flight", c.inFlight.count(), "drain_timeout_ms", c.DrainTimeout.Milliseconds())
	}
	return nil
}

func (c *Consumer) PollOnce(ctx context.Context, handler Handler) error {
	if err := c.reclaim(ctx, handler); err != nil {
		return err
	}

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

// reclaim takes over entries idle past VisibilityTimeout. The ids come from a JUSTID claim, which leaves the
// delivery counter alone, so skipping an entry this process is still running does not push it towards the DLQ.
func (c *Consumer) reclaim(ctx context.Context, handler Handler) error {
	ids, _, err := c.Redis.XAutoClaimJustID(ctx, &redis.XAutoClaimArgs{
		Stream: c.Stream, Group: c.GroupName, Consumer: c.ConsumerName, MinIdle: c.VisibilityTimeout, Start: autoClaimStart, Count: c.BatchSize,
	}).Result()
	if err != nil {
		return errors.ToAppError(fmt.Errorf("xautoclaim: %w", err))
	}
	claimed := c.inFlight.claim(ids)
	if len(claimed) == 0 {
		return nil
	}
	defer c.inFlight.release(claimed...)
	// The entries are already ours and idle was reset by the claim, so no minimum idle time here; XCLAIM counts the delivery.
	msgs, err := c.Redis.XClaim(ctx, &redis.XClaimArgs{Stream: c.Stream, Group: c.GroupName, Consumer: c.ConsumerName, Messages: claimed}).Result()
	if err != nil {
		return errors.ToAppError(fmt.Errorf("xclaim: %w", err))
	}
	for _, raw := range msgs {
		c.handle(ctx, raw, handler)
	}
	return nil
}

// handleEntries marks the whole batch in flight first: entries behind a slow one wait their turn and must not be reclaimed meanwhile.
func (c *Consumer) handleEntries(ctx context.Context, msgs []redis.XMessage, handler Handler) {
	ids := make([]string, len(msgs))
	for i, raw := range msgs {
		ids[i] = raw.ID
	}
	c.inFlight.claim(ids)
	for _, raw := range msgs {
		c.handle(ctx, raw, handler)
		c.inFlight.release(raw.ID)
	}
}

// handle restores the originating request id and tenant, so every log line of the job can be found from the request.
func (c *Consumer) handle(ctx context.Context, raw redis.XMessage, handler Handler) {
	// Once delivered, a job keeps its own context while shutdown stops new reads.
	ctx = context.WithoutCancel(ctx)
	tenantID, _ := raw.Values[StreamFieldTenantID].(string)
	requestID := messageRequestID(raw)
	ctx = requestid.NewContext(tenant.WithID(ctx, tenantID), requestID)
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
		c.observe(metrics.OutcomeSuccess)
		c.Log.InfoContext(ctx, "job handled", "stream", c.Stream, "id", raw.ID, "outbox_id", msg.OutboxID, "deliveries", deliveries, logging.FieldDurationMS, elapsed)
		return
	}
	if deliveries >= int64(c.MaxDeliveries) {
		c.deadLetter(ctx, msg, herr, elapsed)
		return
	}
	c.observe(metrics.OutcomeRetry)
	c.Log.WarnContext(ctx, "job failed, will retry", "stream", c.Stream, "id", raw.ID, "outbox_id", msg.OutboxID, "deliveries", deliveries, "error_code", errors.CodeOf(herr), logging.FieldDurationMS, elapsed)
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
		StreamFieldTenantID: m.TenantID, StreamFieldTraceParent: m.TraceParent,
	}
	if err := c.Redis.XAdd(ctx, &redis.XAddArgs{Stream: c.Stream + DeadLetterSuffix, Values: values}).Err(); err != nil {
		c.Log.ErrorContext(ctx, "dead-letter failed; leaving message pending", "id", m.StreamEntryID, "error_code", errors.CodeOf(err))
		return
	}
	c.ack(ctx, m.StreamEntryID)
	c.Log.ErrorContext(ctx, "job moved to DLQ", "stream", c.Stream, "id", m.StreamEntryID, "outbox_id", m.OutboxID, "deliveries", m.Deliveries, "error_code", errors.CodeOf(cause), logging.FieldDurationMS, elapsed)
	c.observe(metrics.OutcomeDeadLetter)
}

func toMessage(stream string, raw redis.XMessage, requestID string, deliveries int64) Message {
	payload, _ := raw.Values[StreamFieldPayload].(string)
	outboxID, _ := raw.Values[StreamFieldOutboxID].(string)
	tenantID, _ := raw.Values[StreamFieldTenantID].(string)
	traceParent, _ := raw.Values[StreamFieldTraceParent].(string)
	return Message{TenantID: tenantID, OutboxID: outboxID, StreamEntryID: raw.ID, Stream: stream, Payload: []byte(payload), RequestID: requestID, TraceParent: traceParent, Deliveries: deliveries}
}
