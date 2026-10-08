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

func TestLimiterCounterAlwaysHasTTLAndCanReset(t *testing.T) {
	client := startRedis(t)
	limiter := NewLimiter(client, 2, time.Minute)
	require.NoError(t, limiter.Allow(t.Context(), "ttl-test"))
	ttl, err := client.TTL(t.Context(), loginLimiterKeyPrefix+"ttl-test").Result()
	require.NoError(t, err)
	require.Positive(t, ttl, "atomic initialization must never leave a permanent key")
	require.NoError(t, limiter.Reset(t.Context(), "ttl-test"))
	require.Zero(t, client.Exists(t.Context(), loginLimiterKeyPrefix+"ttl-test").Val())
}

func TestLimiterRepairsCounterWithLostTTL(t *testing.T) {
	client := startRedis(t)
	require.NoError(t, client.Set(t.Context(), loginLimiterKeyPrefix+"lost-ttl", 20, 0).Err())
	err := NewLimiter(client, 2, time.Minute).Allow(t.Context(), "lost-ttl")
	require.Equal(t, errors.CodeRateLimited, errors.ToAppError(err).Code)
	require.Positive(t, client.TTL(t.Context(), loginLimiterKeyPrefix+"lost-ttl").Val())
}

func TestLimiterRedisFailureIsReturned(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond, ReadTimeout: 20 * time.Millisecond, WriteTimeout: 20 * time.Millisecond})
	defer client.Close()
	err := NewLimiter(client, 2, time.Minute).Allow(t.Context(), "failure-test")
	require.Error(t, err)
	require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
}

func TestLimiterConcurrentIncrementsAreAtomic(t *testing.T) {
	client := startRedis(t)
	const attempts = 24
	limiter := NewLimiter(client, attempts, time.Minute)
	results := make(chan error, attempts)
	var group sync.WaitGroup
	for range attempts {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- limiter.Allow(t.Context(), "concurrent-test")
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	count, err := client.Get(t.Context(), loginLimiterKeyPrefix+"concurrent-test").Int()
	require.NoError(t, err)
	require.Equal(t, attempts, count)
}

func TestLimiterExpiryFailureIsReturned(t *testing.T) {
	client := startRedis(t)
	failure := stderrors.New("Redis Lua expiry failure")
	client.AddHook(scriptExpiryFailureHook{failure: failure})
	err := NewLimiter(client, 2, time.Minute).Allow(t.Context(), "expiry-test")
	require.ErrorIs(t, err, failure)
	require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
	require.Zero(t, client.Exists(t.Context(), loginLimiterKeyPrefix+"expiry-test").Val(), "failed atomic script cannot leave immortal key")
}

func TestLimiterIdentity_UsesKeyedHMACAndNormalizesEmail(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	lim := NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, key)
	mac := hmac.New(sha256.New, key)
	_, err := mac.Write([]byte("thomas@example.com"))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), lim.Identity(" Thomas@Example.com "))
	other := NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, []byte("different-secret-key-at-least-32-chars"))
	require.NotEqual(t, lim.Identity(ownerEmail), other.Identity(ownerEmail))
	bare := sha256.Sum256([]byte(ownerEmail))
	require.NotEqual(t, hex.EncodeToString(bare[:]), lim.Identity(ownerEmail))
}
