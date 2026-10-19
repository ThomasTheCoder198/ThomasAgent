package redisx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// GroupProbe is ready only when the consumer group exists on the stream, so a worker is not reported ready before it
// can read its jobs.
type GroupProbe struct {
	Client        *redis.Client
	Stream, Group string
}

func (p GroupProbe) Ping(ctx context.Context) error {
	groups, err := p.Client.XInfoGroups(ctx, p.Stream).Result()
	if err != nil {
		return errors.ToAppError(fmt.Errorf("xinfo groups %s: %w", p.Stream, err))
	}
	for _, group := range groups {
		if group.Name == p.Group {
			return nil
		}
	}
	return errors.ToAppError(fmt.Errorf("%w: %s/%s", errors.ErrConsumerGroupMissing, p.Stream, p.Group))
}
