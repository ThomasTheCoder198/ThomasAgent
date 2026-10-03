package redisx

import (
	"context"
	"fmt"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func Open(ctx context.Context, url string) (*redis.Client, error) {
	ctx, span := otel.Tracer("redisx").Start(ctx, "redis.open", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, apperr.From(fmt.Errorf("redis url: %w", err))
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, apperr.From(fmt.Errorf("redis ping: %w", err))
	}
	return client, nil
}

type Pinger struct{ Client *redis.Client }

func (p Pinger) Ping(ctx context.Context) error {
	ctx, span := otel.Tracer("redisx").Start(ctx, "redis.ping", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	return apperr.From(p.Client.Ping(ctx).Err())
}
