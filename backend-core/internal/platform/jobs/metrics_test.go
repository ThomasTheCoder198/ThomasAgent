package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type fakeJobMetrics struct {
	outcomes          []string
	registeredStreams []string
	pending           int
	oldest            time.Duration
}

func (f *fakeJobMetrics) RegisterJobStream(stream string) {
	f.registeredStreams = append(f.registeredStreams, stream)
}

func TestConsumer_RegistersJobStreamBeforeHandling(t *testing.T) {
	ctx := context.Background()
	c := newConsumer(startRedis(t), 1)
	fake := &fakeJobMetrics{}
	c.Metrics = fake
	require.NoError(t, c.EnsureGroup(ctx))
	require.Equal(t, []string{testStream}, fake.registeredStreams)
	require.Empty(t, fake.outcomes)
	require.NoError(t, c.EnsureGroup(ctx))
	require.Equal(t, []string{testStream, testStream}, fake.registeredStreams, "existing groups also register their stream after worker restart")
}

func (f *fakeJobMetrics) ObserveJob(_ string, outcome string) {
	f.outcomes = append(f.outcomes, outcome)
}
func (f *fakeJobMetrics) SetOutboxBacklog(pending int, oldest time.Duration) {
	f.pending, f.oldest = pending, oldest
}

func TestConsumer_RecordsJobOutcomes(t *testing.T) {
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 1)
	fake := &fakeJobMetrics{}
	c.Metrics = fake
	require.NoError(t, c.EnsureGroup(ctx))
	for _, payload := range []string{"ok", "bad"} {
		require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: payload}}).Err())
	}
	require.NoError(t, c.PollOnce(ctx, func(_ context.Context, m Message) error {
		if string(m.Payload) == "bad" {
			return errors.New("poison")
		}
		return nil
	}))
	require.Equal(t, []string{metrics.OutcomeSuccess, metrics.OutcomeDeadLetter}, fake.outcomes)
}

func TestOutboxRelay_RecordsBacklogAfterEachBatch(t *testing.T) {
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	pool, redisClient := startPool(t), startRedis(t)
	require.NoError(t, enqueueTestOutbox(ctx, pool, testStream, map[string]string{"file_id": "f1"}))
	fake := &fakeJobMetrics{pending: -1}
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog, Metrics: fake}
	relay.recordBacklog(ctx)
	require.Equal(t, 1, fake.pending)
	_, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	relay.recordBacklog(ctx)
	require.Equal(t, 0, fake.pending)
	require.Zero(t, fake.oldest)
}
