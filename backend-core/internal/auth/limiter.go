package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const limiterKeyPrefix = "thomas:auth:login:"
const (
	limiterEmailSuffix   = ":email"
	limiterIPSuffix      = ":ip:"
	retryAfterSeconds    = "retryAfterSeconds"
	limiterResultLength  = 3
	limiterTTLIndex      = 1
	limiterTripIndex     = 2
	limiterEmailKeyIndex = 2
	limiterIPKeyPrefix   = limiterKeyPrefix + "ip:"
)

const incrementWithTTL = `
local blocked = 0
local remaining = 0
local email_tripped = 0
local window_arg = 1
local email_key_arg = window_arg + 1
local email_key_index = tonumber(ARGV[email_key_arg])
for index, key in ipairs(KEYS) do
  local count = redis.call('INCR', key)
  local ttl = redis.call('PTTL', key)
  if count == 1 or ttl < 0 then
    redis.call('PEXPIRE', key, ARGV[window_arg])
    ttl = tonumber(ARGV[window_arg])
  end
  local limit = tonumber(ARGV[index + email_key_arg])
  if count > limit then
    blocked = 1
    remaining = math.max(remaining, ttl)
  end
  if index == email_key_index and count == limit + 1 then email_tripped = 1 end
end
return {blocked, remaining, email_tripped}
`

type Limiter struct {
	rdb      *redis.Client
	max      int
	window   time.Duration
	script   *redis.Script
	emailMax int
	ipMax    int
	hmacKey  []byte
	initErr  error
}

func NewLimiter(rdb *redis.Client, max int, window time.Duration) *Limiter {
	key := make([]byte, tokenBytes)
	_, err := rand.Read(key)
	lim := NewLoginLimiter(rdb, config.AuthConfig{LoginMaxAttempts: max, LoginEmailMaxAttempts: max, LoginIPMaxAttempts: config.DefaultLoginIPMaxAttempts, LoginWindow: window}, key)
	if err != nil {
		lim.initErr = errors.ErrInternalError.WithCause(err)
	}
	return lim
}

func NewLoginLimiter(rdb *redis.Client, cfg config.AuthConfig, key []byte) *Limiter {
	lim := &Limiter{rdb: rdb, max: cfg.LoginMaxAttempts, emailMax: cfg.LoginEmailMaxAttempts, ipMax: cfg.LoginIPMaxAttempts, window: cfg.LoginWindow, script: redis.NewScript(incrementWithTTL), hmacKey: append([]byte(nil), key...)}
	if len(key) < config.MinimumServiceTokenLength || lim.max < 1 || lim.emailMax < 1 || lim.ipMax < 1 || lim.window <= 0 {
		lim.initErr = errors.ErrInternalError
	}
	return lim
}

func (l *Limiter) Allow(ctx context.Context, key string) error {
	_, err := l.checkKeys(ctx, []string{limiterKeyPrefix + key}, 0, l.max)
	return err
}

type LimitDecision struct{ EmailTripped bool }

func (l *Limiter) Check(ctx context.Context, email, ip string) (LimitDecision, error) {
	identity := l.identity(email)
	return l.checkKeys(ctx, []string{limiterKeyPrefix + identity + limiterIPSuffix + ip, limiterKeyPrefix + identity + limiterEmailSuffix, limiterIPKeyPrefix + ip}, limiterEmailKeyIndex, l.max, l.emailMax, l.ipMax)
}

func (l *Limiter) checkKeys(ctx context.Context, keys []string, emailKeyIndex int, limits ...int) (LimitDecision, error) {
	ctx, span := otel.Tracer(TracerName).Start(ctx, limiterCheckSpan)
	defer span.End()
	if l.initErr != nil {
		return LimitDecision{}, l.initErr
	}
	args := []any{max(l.window.Milliseconds(), 1), emailKeyIndex}
	for _, limit := range limits {
		args = append(args, limit)
	}
	result, err := l.script.Run(ctx, l.rdb, keys, args...).Int64Slice()
	if err != nil {
		span.RecordError(err)
		return LimitDecision{}, errors.ErrInternalError.WithCause(fmt.Errorf("login limiter increment: %w", err))
	}
	if len(result) != limiterResultLength {
		return LimitDecision{}, errors.ErrInternalError
	}
	decision := LimitDecision{EmailTripped: result[limiterTripIndex] == 1}
	if result[0] == 1 {
		remaining := max(int64(math.Ceil(float64(result[limiterTTLIndex])/float64(time.Second/time.Millisecond))), 1)
		return decision, errors.ErrRateLimited.WithDetails(map[string]any{retryAfterSeconds: remaining})
	}
	return decision, nil
}

func (l *Limiter) Reset(ctx context.Context, key string) error {
	ctx, span := otel.Tracer(TracerName).Start(ctx, limiterResetSpan)
	defer span.End()
	if err := l.rdb.Del(ctx, limiterKeyPrefix+key).Err(); err != nil {
		return errors.ErrInternalError.WithCause(fmt.Errorf("login limiter reset: %w", err))
	}
	return nil
}

func (l *Limiter) ResetLogin(ctx context.Context, email, ip string) error {
	ctx, span := otel.Tracer(TracerName).Start(ctx, limiterResetSpan)
	defer span.End()
	identity := l.identity(email)
	if err := l.rdb.Del(ctx, limiterKeyPrefix+identity+limiterIPSuffix+ip, limiterKeyPrefix+identity+limiterEmailSuffix).Err(); err != nil {
		span.RecordError(err)
		return errors.ErrInternalError.WithCause(err)
	}
	return nil
}

func (l *Limiter) identity(email string) string {
	mac := hmac.New(sha256.New, l.hmacKey)
	_, _ = mac.Write([]byte(normalizeEmail(email)))
	return hex.EncodeToString(mac.Sum(nil))
}
