package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const (
	loginLimiterKeyPrefix   = "thomas:auth:login:"
	limiterEmailSuffix      = ":email"
	limiterIPSuffix         = ":ip:"
	limiterIPKeyPrefix      = loginLimiterKeyPrefix + "ip:"
	retryAfterSecondsDetail = "retryAfterSeconds"
)

// Positions in the script's reply and in its KEYS (1-based in Lua).
const (
	scriptResultLength   = 3
	scriptBlockedIndex   = 0
	scriptTTLIndex       = 1
	scriptTrippedIndex   = 2
	scriptEmailKeyNumber = 2
)

// incrementWithTTL bumps every counter atomically, repairs a counter that lost its TTL, and
// reports whether any counter is over its limit, the longest remaining TTL among the blocked
// ones, and whether this attempt is the first one over the email limit.
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

// LoginLimiter keeps three Redis budgets per login attempt: email+IP, email alone, and IP alone.
// Emails are stored only as keyed HMACs.
type LoginLimiter struct {
	redisClient          *redis.Client
	script               *redis.Script
	emailAndIPMax        int
	emailMax             int
	ipMax                int
	window               time.Duration
	identityKey          []byte
	configurationInvalid bool
}

// LoginLimitDecision tells the controller whether this attempt is the first over the email budget,
// which is the only failed-login event worth an audit row.
type LoginLimitDecision struct{ EmailTripped bool }

type loginLimitKeys struct{ emailAndIP, email, ip string }

func NewLoginLimiter(redisClient *redis.Client, authConfig config.AuthConfig, identityKey []byte) *LoginLimiter {
	limiter := &LoginLimiter{
		redisClient:   redisClient,
		script:        redis.NewScript(incrementWithTTL),
		emailAndIPMax: authConfig.LoginMaxAttempts,
		emailMax:      authConfig.LoginEmailMaxAttempts,
		ipMax:         authConfig.LoginIPMaxAttempts,
		window:        authConfig.LoginWindow,
		identityKey:   append([]byte(nil), identityKey...),
	}
	limiter.configurationInvalid = len(identityKey) < config.MinimumServiceTokenLength ||
		limiter.emailAndIPMax < 1 || limiter.emailMax < 1 || limiter.ipMax < 1 || limiter.window <= 0
	return limiter
}

// Identity is the keyed HMAC of the normalized email; it is safe to log.
func (l *LoginLimiter) Identity(email string) string {
	mac := hmac.New(sha256.New, l.identityKey)
	_, _ = mac.Write([]byte(NormalizeEmail(email)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (l *LoginLimiter) keysFor(email, ip string) loginLimitKeys {
	identity := l.Identity(email)
	return loginLimitKeys{
		emailAndIP: loginLimiterKeyPrefix + identity + limiterIPSuffix + ip,
		email:      loginLimiterKeyPrefix + identity + limiterEmailSuffix,
		ip:         limiterIPKeyPrefix + ip,
	}
}

// Check counts one attempt and returns RATE_LIMITED, with retryAfterSeconds, when any budget is spent.
func (l *LoginLimiter) Check(ctx context.Context, email, ip string) (LoginLimitDecision, error) {
	if l.configurationInvalid {
		return LoginLimitDecision{}, errors.ErrInternalError
	}
	keys := l.keysFor(email, ip)
	reply, err := l.script.Run(ctx, l.redisClient, []string{keys.emailAndIP, keys.email, keys.ip},
		max(l.window.Milliseconds(), 1), scriptEmailKeyNumber, l.emailAndIPMax, l.emailMax, l.ipMax).Int64Slice()
	if err != nil {
		return LoginLimitDecision{}, errors.ErrInternalError.WithCause(fmt.Errorf("login limiter increment: %w", err))
	}
	if len(reply) != scriptResultLength {
		return LoginLimitDecision{}, errors.ErrInternalError
	}
	decision := LoginLimitDecision{EmailTripped: reply[scriptTrippedIndex] == 1}
	if reply[scriptBlockedIndex] == 1 {
		retryAfterSeconds := max(int64(math.Ceil(float64(reply[scriptTTLIndex])/float64(time.Second/time.Millisecond))), 1)
		return decision, errors.ErrRateLimited.WithDetails(map[string]any{retryAfterSecondsDetail: retryAfterSeconds})
	}
	return decision, nil
}

// ResetLogin clears the email budgets after a successful login; the IP budget stays until it expires.
func (l *LoginLimiter) ResetLogin(ctx context.Context, email, ip string) error {
	keys := l.keysFor(email, ip)
	if err := l.redisClient.Del(ctx, keys.emailAndIP, keys.email).Err(); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	return nil
}
