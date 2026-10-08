package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const limiterTestIP = "192.0.2.10"

var testLimiterIdentityKey = []byte("0123456789abcdef0123456789abcdef")

// newTestLimiter gives the email+IP and email budgets the same limit and keeps the default IP budget.
func newTestLimiter(redisClient *redis.Client, maxAttempts int) *LoginLimiter {
	return NewLoginLimiter(redisClient, config.AuthConfig{
		LoginMaxAttempts:      maxAttempts,
		LoginEmailMaxAttempts: maxAttempts,
		LoginIPMaxAttempts:    config.DefaultLoginIPMaxAttempts,
		LoginWindow:           time.Minute,
	}, testLimiterIdentityKey)
}

func TestLoginLimiter_CountersAlwaysHaveTTLAndSuccessResetsEmailBudgets(t *testing.T) {
	client := startRedis(t)
	limiter := newTestLimiter(client, 2)
	keys := limiter.keysFor(ownerEmail, limiterTestIP)
	_, err := limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.NoError(t, err)
	for _, key := range []string{keys.emailAndIP, keys.email, keys.ip} {
		require.Positive(t, client.TTL(t.Context(), key).Val(), "atomic initialization must never leave a permanent key: "+key)
	}
	require.NoError(t, limiter.ResetLogin(t.Context(), ownerEmail, limiterTestIP))
	require.Zero(t, client.Exists(t.Context(), keys.emailAndIP, keys.email).Val())
	require.Equal(t, int64(1), client.Exists(t.Context(), keys.ip).Val(), "the IP budget outlives a successful login")
}

func TestLoginLimiter_RepairsCounterWithLostTTL(t *testing.T) {
	client := startRedis(t)
	limiter := newTestLimiter(client, 2)
	keys := limiter.keysFor(ownerEmail, limiterTestIP)
	require.NoError(t, client.Set(t.Context(), keys.emailAndIP, 20, 0).Err())
	_, err := limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.Equal(t, errors.CodeRateLimited, errors.ToAppError(err).Code)
	require.Positive(t, client.TTL(t.Context(), keys.emailAndIP).Val())
}

func TestLoginLimiter_RedisFailureIsReturned(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond, ReadTimeout: 20 * time.Millisecond, WriteTimeout: 20 * time.Millisecond})
	defer client.Close()
	_, err := newTestLimiter(client, 2).Check(t.Context(), ownerEmail, limiterTestIP)
	require.Error(t, err)
	require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
}

func TestLoginLimiter_ConcurrentIncrementsAreAtomic(t *testing.T) {
	client := startRedis(t)
	const attempts = 24
	limiter := newTestLimiter(client, attempts)
	results := make(chan error, attempts)
	var group sync.WaitGroup
	for range attempts {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := limiter.Check(t.Context(), ownerEmail, limiterTestIP)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	count, err := client.Get(t.Context(), limiter.keysFor(ownerEmail, limiterTestIP).emailAndIP).Int()
	require.NoError(t, err)
	require.Equal(t, attempts, count)
}

func TestLoginLimiter_ScriptFailureIsReturnedWithoutLeavingKeys(t *testing.T) {
	client := startRedis(t)
	failure := stderrors.New("Redis Lua expiry failure")
	client.AddHook(scriptExpiryFailureHook{failure: failure})
	limiter := newTestLimiter(client, 2)
	_, err := limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.ErrorIs(t, err, failure)
	require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
	require.Zero(t, client.Exists(t.Context(), limiter.keysFor(ownerEmail, limiterTestIP).email).Val(), "failed atomic script cannot leave immortal key")
}

func TestLoginLimiter_RetryAfterUsesRemainingTTL(t *testing.T) {
	client := startRedis(t)
	limiter := newTestLimiter(client, 1)
	keys := limiter.keysFor(ownerEmail, limiterTestIP)
	_, err := limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.NoError(t, err)
	expireEmailBudgets := func(ttl time.Duration) {
		for _, key := range []string{keys.emailAndIP, keys.email} {
			require.NoError(t, client.PExpire(t.Context(), key, ttl).Err())
		}
	}
	expireEmailBudgets(20 * time.Second)
	_, err = limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.Equal(t, errors.CodeRateLimited, errors.ToAppError(err).Code)
	require.EqualValues(t, 20, errors.ToAppError(err).Details[retryAfterSecondsDetail])
	expireEmailBudgets(7 * time.Second)
	_, err = limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.EqualValues(t, 7, errors.ToAppError(err).Details[retryAfterSecondsDetail])
}

func TestLoginLimiter_InvalidConfigurationRejectsEveryAttempt(t *testing.T) {
	limiter := NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 1, LoginIPMaxAttempts: 1, LoginWindow: time.Minute}, []byte("short"))
	_, err := limiter.Check(t.Context(), ownerEmail, limiterTestIP)
	require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
}

func TestLoginLimiter_IdentityUsesKeyedHMACAndNormalizesEmail(t *testing.T) {
	limiter := newTestLimiter(nil, 1)
	mac := hmac.New(sha256.New, testLimiterIdentityKey)
	_, err := mac.Write([]byte("thomas@example.com"))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), limiter.Identity(" Thomas@Example.com "))
	other := NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, []byte("different-secret-key-at-least-32-chars"))
	require.NotEqual(t, limiter.Identity(ownerEmail), other.Identity(ownerEmail))
	bare := sha256.Sum256([]byte(ownerEmail))
	require.NotEqual(t, hex.EncodeToString(bare[:]), limiter.Identity(ownerEmail))
}
