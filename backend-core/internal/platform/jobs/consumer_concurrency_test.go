package jobs

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestConsumerRun_ConcurrencyAndAckAfterHandler(t *testing.T) {
	client := startRedis(t)
	consumer := newConsumer(client, 3)
	consumer.Concurrency = 3
	consumer.VisibilityTimeout = time.Minute
	consumer.BatchSize = 1
	consumer.DrainTimeout = time.Second
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	for range 4 {
		require.NoError(t, client.XAdd(t.Context(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "{}"}}).Err())
	}
	var active, peak atomic.Int32
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		_ = consumer.Run(ctx, func(context.Context, Message) error {
			now := active.Add(1)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			<-release
			active.Add(-1)
			return nil
		})
		close(done)
	}()
	require.Eventually(t, func() bool { return peak.Load() == 3 }, time.Second, 5*time.Millisecond)
	pending, err := client.XPending(t.Context(), testStream, testGroup).Result()
	require.NoError(t, err)
	require.EqualValues(t, 3, pending.Count)
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consumer did not drain")
	}
	require.EqualValues(t, 3, peak.Load())
}

func TestConsumerRun_DrainTimeout(t *testing.T) {
	client := startRedis(t)
	consumer := newConsumer(client, 3)
	consumer.VisibilityTimeout = time.Minute
	consumer.DrainTimeout = 30 * time.Millisecond
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	require.NoError(t, client.XAdd(t.Context(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "{}"}}).Err())
	entered := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		_ = consumer.Run(ctx, func(context.Context, Message) error { close(entered); <-release; return nil })
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain timeout ignored")
	}
	close(release)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestConsumerRun_WaitsForInFlightHandlerThenReturns(t *testing.T) {
	client := startRedis(t)
	consumer := newConsumer(client, 3)
	consumer.VisibilityTimeout = time.Minute
	consumer.DrainTimeout = time.Minute
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	require.NoError(t, client.XAdd(t.Context(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "{}"}}).Err())
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		_ = consumer.Run(ctx, func(context.Context, Message) error { close(entered); <-release; return nil })
		close(done)
	}()
	<-entered
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a handler was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the handler finished")
	}
	pending, err := client.XPending(context.Background(), testStream, testGroup).Result()
	require.NoError(t, err)
	require.Zero(t, pending.Count, "the handler finished during the drain, so its entry is acknowledged")
}

func TestConsumerRun_DrainTimeoutWarnsWithInFlightCount(t *testing.T) {
	client := startRedis(t)
	out := &syncBuffer{}
	consumer := newConsumer(client, 3)
	consumer.Log = slog.New(slog.NewTextHandler(out, nil))
	consumer.VisibilityTimeout = time.Minute
	consumer.DrainTimeout = 30 * time.Millisecond
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	require.NoError(t, client.XAdd(t.Context(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "{}"}}).Err())
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		_ = consumer.Run(ctx, func(context.Context, Message) error { close(entered); <-release; return nil })
		close(done)
	}()
	<-entered
	cancel()
	<-done
	require.Contains(t, out.String(), "level=WARN")
	require.Contains(t, out.String(), "in_flight=1")
	close(release)
}

func TestConsumerRun_ConcurrentWorkersDoNotReclaimEachOthersInFlightEntries(t *testing.T) {
	client := startRedis(t)
	consumer := newConsumer(client, 10)
	consumer.Concurrency = 3
	consumer.BatchSize = 1
	consumer.BlockTimeout = 5 * time.Millisecond
	// Shorter than the handler: the sibling pollers see the running entry as idle and would claim it.
	consumer.VisibilityTimeout = 20 * time.Millisecond
	consumer.DrainTimeout = time.Second
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	require.NoError(t, client.XAdd(t.Context(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "{}"}}).Err())
	var runs atomic.Int32
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		_ = consumer.Run(ctx, func(context.Context, Message) error { runs.Add(1); <-release; return nil })
		close(done)
	}()
	require.Eventually(t, func() bool { return runs.Load() >= 1 }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	require.EqualValues(t, 1, runs.Load(), "an entry a handler is still running must not run again in a sibling poller")
	cancel()
	close(release)
	<-done
	pending, err := client.XPending(context.Background(), testStream, testGroup).Result()
	require.NoError(t, err)
	require.Zero(t, pending.Count)
}

func TestConsumerRun_StillReclaimsIdleEntriesOfDeadConsumers(t *testing.T) {
	client := startRedis(t)
	consumer := newConsumer(client, 10)
	consumer.ConsumerName = "survivor"
	consumer.VisibilityTimeout = 20 * time.Millisecond
	consumer.BatchSize = 1
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	require.NoError(t, client.XAdd(t.Context(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "{}"}}).Err())
	// A crashed worker read the entry and never acknowledged it.
	require.NoError(t, client.XReadGroup(t.Context(), &redis.XReadGroupArgs{Group: testGroup, Consumer: "crashed", Streams: []string{testStream, ">"}, Count: 1}).Err())
	time.Sleep(50 * time.Millisecond)
	var deliveries atomic.Int64
	require.NoError(t, consumer.PollOnce(t.Context(), func(_ context.Context, m Message) error { deliveries.Store(m.Deliveries); return nil }))
	require.EqualValues(t, 2, deliveries.Load(), "a reclaimed entry counts as a second delivery")
}
