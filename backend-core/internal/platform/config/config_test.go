package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CORE_DATABASE_URL", "postgres://u:p@localhost:5432/thomas")
	t.Setenv("CORE_REDIS_URL", "redis://localhost:6379/0")
}

func TestLoadAppliesDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 15*time.Second, cfg.ShutdownTimeout)
	require.Equal(t, 4, cfg.Retry.MaxAttempts)
	require.Equal(t, 5, cfg.Stream.MaxDeliveries)
	require.Equal(t, 30*time.Second, cfg.Stream.VisibilityTimeout)
	require.Equal(t, "info", cfg.LogLevel)
}

func TestLoadFailsWithoutRequired(t *testing.T) {
	t.Setenv("CORE_DATABASE_URL", "")
	t.Setenv("CORE_REDIS_URL", "")
	_, err := Load()
	require.Error(t, err)
}

func TestLoadRejectsUnknownLogLevel(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_LOG_LEVEL", "verbose")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_LOG_LEVEL")
	require.ErrorContains(t, err, "unknown log level")
}

func TestLoadAcceptsKnownLogLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CORE_LOG_LEVEL", level)
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, level, cfg.LogLevel)
		})
	}
}

func TestLoadRejectsMaxDelayBelowBaseDelay(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_RETRY_BASE_DELAY", "2s")
	t.Setenv("CORE_RETRY_MAX_DELAY", "1s")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_RETRY_MAX_DELAY")
}

func TestLoadBreakerDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, uint32(5), cfg.Breaker.FailureThreshold)
	require.Equal(t, 30*time.Second, cfg.Breaker.OpenTimeout)
	require.Equal(t, uint32(1), cfg.Breaker.HalfOpenMaxCalls)
}

func TestLoadBreakerOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_BREAKER_FAILURE_THRESHOLD", "7")
	t.Setenv("CORE_BREAKER_OPEN_TIMEOUT", "45s")
	t.Setenv("CORE_BREAKER_HALF_OPEN_MAX_CALLS", "3")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, uint32(7), cfg.Breaker.FailureThreshold)
	require.Equal(t, 45*time.Second, cfg.Breaker.OpenTimeout)
	require.Equal(t, uint32(3), cfg.Breaker.HalfOpenMaxCalls)
}

func TestLoadRejectsZeroBreakerLimits(t *testing.T) {
	for _, name := range []string{"CORE_BREAKER_FAILURE_THRESHOLD", "CORE_BREAKER_HALF_OPEN_MAX_CALLS"} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, "0")
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
}
