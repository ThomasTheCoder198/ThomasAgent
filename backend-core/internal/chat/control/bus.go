package control

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	stopKeyPrefix    = "run-stop:"
	channelKeyPrefix = "run-control:"
	// stopValue is the stop key's value and the control channel's message: only their existence matters.
	stopValue   = "1"
	stopMessage = "stop"
)

func stopKey(runID uuid.UUID) string    { return stopKeyPrefix + runID.String() }
func channelKey(runID uuid.UUID) string { return channelKeyPrefix + runID.String() }

// Bus carries Stop from the API to the worker that runs a run, on any replica (amendment W6). The key survives a
// message nobody was listening for; the publish wakes a listener at once. Keys are per run id, so a stop never
// reaches a later run. 2b-6 adds Steer on the same subscription.
type Bus struct{ redis *redis.Client }

func NewBus(client *redis.Client) *Bus { return &Bus{redis: client} }

// Stop asks the run to stop. The key lives until `until` (deadline_at + finish timeout) and is set before the publish,
// so a worker that subscribes late finds it. Stopping twice, or a run that already ended, is not an error.
func (b *Bus) Stop(ctx context.Context, runID uuid.UUID, until time.Time) error {
	if err := b.redis.SetArgs(ctx, stopKey(runID), stopValue, redis.SetArgs{ExpireAt: until}).Err(); err != nil {
		return errors.ToAppError(fmt.Errorf("set run stop key: %w", err))
	}
	if err := b.redis.Publish(ctx, channelKey(runID), stopMessage).Err(); err != nil {
		return errors.ToAppError(fmt.Errorf("publish run stop: %w", err))
	}
	return nil
}

// IsStopped reports whether a stop key exists for the run.
func (b *Bus) IsStopped(ctx context.Context, runID uuid.UUID) (bool, error) {
	count, err := b.redis.Exists(ctx, stopKey(runID)).Result()
	if err != nil {
		return false, errors.ToAppError(fmt.Errorf("read run stop key: %w", err))
	}
	return count > 0, nil
}

// Subscription is what a running executor holds for its lifetime. Close it when the run ends.
type Subscription interface {
	// Stopped fires once: on a published stop, or when a key check finds the stop key.
	Stopped() <-chan struct{}
	// Check reads the stop key; the executor calls it between steps and before each tool batch.
	Check(ctx context.Context) (bool, error)
	Close() error
}

// Subscribe listens to the run's control channel first, waits for Redis to confirm the subscription, and only then
// checks the stop key: a stop published before the subscription is found by the key, one published after by the
// channel, and none can fall between them.
func (b *Bus) Subscribe(ctx context.Context, runID uuid.UUID) (Subscription, error) {
	pubsub := b.redis.Subscribe(ctx, channelKey(runID))
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("%w: %w", errors.ErrRunControlInvalid, err)
	}
	sub := &subscription{bus: b, runID: runID, pubsub: pubsub, stopped: make(chan struct{}), done: make(chan struct{})}
	go sub.listen()
	if _, err := sub.Check(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	return sub, nil
}

type subscription struct {
	bus       *Bus
	runID     uuid.UUID
	pubsub    *redis.PubSub
	stopped   chan struct{}
	fireOnce  sync.Once
	closeOnce sync.Once
	done      chan struct{}
}

func (s *subscription) Stopped() <-chan struct{} { return s.stopped }

func (s *subscription) fire() { s.fireOnce.Do(func() { close(s.stopped) }) }

func (s *subscription) listen() {
	messages := s.pubsub.Channel()
	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			if message.Payload == stopMessage {
				s.fire()
			}
		case <-s.done:
			return
		}
	}
}

func (s *subscription) Check(ctx context.Context) (bool, error) {
	stopped, err := s.bus.IsStopped(ctx, s.runID)
	if err != nil {
		return false, err
	}
	if stopped {
		s.fire()
	}
	return stopped, nil
}

func (s *subscription) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		err = s.pubsub.Close()
	})
	return err
}
