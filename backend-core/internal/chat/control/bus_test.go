package control

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx/redistest"
)

const fireWait = 5 * time.Second

func fired(sub Subscription) bool {
	select {
	case <-sub.Stopped():
		return true
	default:
		return false
	}
}

func waitFired(t *testing.T, sub Subscription) {
	t.Helper()
	select {
	case <-sub.Stopped():
	case <-time.After(fireWait):
		t.Fatal("Stopped did not fire")
	}
}

func subscribers(t *testing.T, client *redis.Client, runID uuid.UUID) int64 {
	t.Helper()
	counts, err := client.PubSubNumSub(context.Background(), channelKey(runID)).Result()
	require.NoError(t, err)
	return counts[channelKey(runID)]
}

func TestBus_StopReachesASubscriberThroughThePublish(t *testing.T) {
	client := redistest.Start(t)
	bus := NewBus(client)
	runID := uuid.New()
	sub, err := bus.Subscribe(context.Background(), runID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })
	require.False(t, fired(sub), "nothing stopped yet")

	require.NoError(t, bus.Stop(context.Background(), runID, time.Now().Add(time.Minute)))
	waitFired(t, sub)
	ttl, err := client.TTL(context.Background(), stopKey(runID)).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 30*time.Second, "the stop key lives until the given time")
}

func TestBus_StopPublishedBeforeSubscribeIsStillSeen(t *testing.T) {
	client := redistest.Start(t)
	bus := NewBus(client)
	runID := uuid.New()
	require.NoError(t, bus.Stop(context.Background(), runID, time.Now().Add(time.Minute)))

	sub, err := bus.Subscribe(context.Background(), runID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })
	require.True(t, fired(sub), "the key is checked right after the subscription is confirmed")
}

func TestBus_CheckSeesAStopWhoseMessageWasLost(t *testing.T) {
	client := redistest.Start(t)
	bus := NewBus(client)
	runID := uuid.New()
	sub, err := bus.Subscribe(context.Background(), runID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })

	stopped, err := sub.Check(context.Background())
	require.NoError(t, err)
	require.False(t, stopped)
	require.NoError(t, client.Set(context.Background(), stopKey(runID), "1", time.Minute).Err(), "the key without any publish")
	stopped, err = sub.Check(context.Background())
	require.NoError(t, err)
	require.True(t, stopped)
	require.True(t, fired(sub), "Check also wakes anyone waiting on Stopped")
}

func TestBus_StopOfOneRunNeverAffectsAnother(t *testing.T) {
	client := redistest.Start(t)
	bus := NewBus(client)
	other, stopped := uuid.New(), uuid.New()
	sub, err := bus.Subscribe(context.Background(), other)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })
	require.NoError(t, bus.Stop(context.Background(), stopped, time.Now().Add(time.Minute)))
	time.Sleep(200 * time.Millisecond)
	require.False(t, fired(sub))
	check, err := sub.Check(context.Background())
	require.NoError(t, err)
	require.False(t, check)
}

func TestBus_StopIsIdempotentAndASpentKeyExpires(t *testing.T) {
	client := redistest.Start(t)
	bus := NewBus(client)
	runID := uuid.New()
	until := time.Now().Add(time.Minute)
	require.NoError(t, bus.Stop(context.Background(), runID, until))
	require.NoError(t, bus.Stop(context.Background(), runID, until))
	stopped, err := bus.IsStopped(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, stopped)

	// A run that already ended asks to stop with a time in the past: the key is gone at once, no error.
	ended := uuid.New()
	require.NoError(t, bus.Stop(context.Background(), ended, time.Now().Add(-time.Second)))
	stopped, err = bus.IsStopped(context.Background(), ended)
	require.NoError(t, err)
	require.False(t, stopped)
}

func TestBus_CloseReleasesTheSubscription(t *testing.T) {
	client := redistest.Start(t)
	bus := NewBus(client)
	runID := uuid.New()
	sub, err := bus.Subscribe(context.Background(), runID)
	require.NoError(t, err)
	require.EqualValues(t, 1, subscribers(t, client, runID))
	require.NoError(t, sub.Close())
	require.NoError(t, sub.Close(), "Close is idempotent")
	require.Eventually(t, func() bool { return subscribers(t, client, runID) == 0 }, fireWait, 50*time.Millisecond)
}

func TestBus_SubscribeFailsWhenRedisIsDown(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { _ = dead.Close() })
	_, err := NewBus(dead).Subscribe(context.Background(), uuid.New())
	require.Error(t, err)
}
