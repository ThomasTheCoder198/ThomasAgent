package redisx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func Open(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.ToAppError(fmt.Errorf("redis url: %w", err))
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.ToAppError(fmt.Errorf("redis ping: %w", err))
	}
	return client, nil
}

type ClientPinger struct{ Client *redis.Client }

func (p ClientPinger) Ping(ctx context.Context) error {
	if err := p.Client.Ping(ctx).Err(); err != nil {
		return errors.ToAppError(err)
	}
	return nil
}
