package runstream

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	streamStart = "0"
	// readBatch bounds the entries one XREAD returns; the loop reads again at once, so it only limits memory.
	readBatch = 100
	// minBlock keeps BLOCK positive: go-redis sends BLOCK 0 for a zero Block, which blocks forever.
	minBlock = time.Millisecond
)

// StatusFunc reads the run row's status, in the caller's tenant and user scope. A failure is not fatal: the relay
// keeps following the stream and asks again at the next check.
type StatusFunc func(ctx context.Context, runID uuid.UUID) (convmodel.RunStatus, error)

// Reader follows a run's stream for the API relay (amendment W4).
type Reader struct {
	Redis *redis.Client
	// BlockTimeout is CORE_STREAM_BLOCK_TIMEOUT: the longest one XREAD waits.
	BlockTimeout time.Duration
	// StatusCheck is CORE_CHAT_RELAY_STATUS_CHECK: how often the row is consulted while waiting.
	StatusCheck time.Duration
	Status      StatusFunc
}

// follower is the state of one Follow call.
type follower struct {
	*Reader
	key       string
	last      string
	deadline  time.Time
	nextCheck time.Time
	rowEnded  bool
}

// Follow relays events from the start of the stream to onEvent until the worker's terminal event, the end of the
// window (deadline = deadline_at + finish timeout), a row that is no longer running, or an error. An absent key
// means "not started yet", never "ended". Events are read from 0, so a late reader sees the whole run.
//
// A Redis failure returns NoTerminal with the error: the caller then falls back to the database through RelayEnd.
// A cancelled ctx (the client left) returns NoTerminal and the ctx error. An error from onEvent stops the relay.
func (r *Reader) Follow(ctx context.Context, runID uuid.UUID, deadline time.Time, onEvent func(model.Event) error) (Outcome, error) {
	f := &follower{Reader: r, key: StreamKey(runID), last: streamStart, deadline: deadline, nextCheck: time.Now().Add(r.StatusCheck)}
	for {
		if err := ctx.Err(); err != nil {
			return NoTerminal, err
		}
		if time.Until(deadline) <= 0 {
			return NoTerminal, nil
		}
		finalBlock := f.rowEnded
		streams, err := r.Redis.XRead(ctx, &redis.XReadArgs{Streams: []string{f.key, f.last}, Count: readBatch, Block: f.block()}).Result()
		switch {
		case err == nil:
			outcome, done, relayErr := f.relayBatch(ctx, runID, streams, onEvent, finalBlock)
			if done {
				return outcome, relayErr
			}
		case stderrors.Is(err, redis.Nil):
			if f.rowEnded {
				return RowEnded, nil
			}
			f.checkRow(ctx, runID)
		case ctx.Err() != nil:
			return NoTerminal, ctx.Err()
		default:
			return NoTerminal, errors.ToAppError(fmt.Errorf("xread run stream: %w", err))
		}
	}
}

// relayBatch checks the row even while events continue, then permits one final read for a late terminal event.
func (f *follower) relayBatch(ctx context.Context, runID uuid.UUID, streams []redis.XStream,
	onEvent func(model.Event) error, finalBlock bool) (Outcome, bool, error) {
	outcome, done, err := f.relay(streams, onEvent)
	if err != nil || done {
		return outcome, true, err
	}
	if finalBlock {
		return RowEnded, true, nil
	}
	f.checkRow(ctx, runID)
	return "", false, nil
}

// block is how long the next XREAD may wait: never past the window, never past the next status check, never past
// CORE_STREAM_BLOCK_TIMEOUT. Once the row ended, the terminal event gets one more block to arrive (the worker writes it
// right after the commit); waiting for the next status check would never end.
func (f *follower) block() time.Duration {
	block := min(f.BlockTimeout, time.Until(f.deadline))
	if !f.rowEnded {
		block = min(block, time.Until(f.nextCheck))
	}
	return max(block, minBlock)
}

// checkRow asks for the row's status when a check is due.
func (f *follower) checkRow(ctx context.Context, runID uuid.UUID) {
	if f.rowEnded || time.Now().Before(f.nextCheck) {
		return
	}
	f.nextCheck = time.Now().Add(f.StatusCheck)
	if f.Status == nil {
		return
	}
	status, err := f.Status(ctx, runID)
	f.rowEnded = err == nil && status != convmodel.RunStatusRunning
}

// relay delivers one XREAD result; done is true after the terminal event.
func (f *follower) relay(streams []redis.XStream, onEvent func(model.Event) error) (outcome Outcome, done bool, err error) {
	for _, stream := range streams {
		for _, message := range stream.Messages {
			f.last = message.ID
			raw, _ := message.Values[FieldEvent].(string)
			event, err := DecodeEvent([]byte(raw))
			if err != nil {
				return NoTerminal, true, err
			}
			if err := onEvent(event); err != nil {
				return NoTerminal, true, err
			}
			if IsTerminal(event) {
				return TerminalRelayed, true, nil
			}
		}
	}
	return "", false, nil
}
