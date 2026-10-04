package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CORE_VAULT_MASTER_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("CORE_DATABASE_URL", "postgres://u:p@localhost:5432/thomas")
	t.Setenv("CORE_REDIS_URL", "redis://localhost:6379/0")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 15*time.Second, cfg.ShutdownTimeout)
	require.Equal(t, 4, cfg.Retry.MaxAttempts)
	require.Equal(t, 4, cfg.Stream.MaxDeliveries)
	require.Equal(t, 30*time.Second, cfg.Stream.VisibilityTimeout)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, 100, cfg.OutboxRelay.BatchSize)
	require.Equal(t, 500*time.Millisecond, cfg.OutboxRelay.PollInterval)
}

func TestLoad_FailsWithoutRequired(t *testing.T) {
	t.Setenv("CORE_DATABASE_URL", "")
	t.Setenv("CORE_REDIS_URL", "")
	_, err := Load()
	require.Error(t, err)
}

func TestLoad_RejectsUnknownLogLevel(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_LOG_LEVEL", "verbose")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_LOG_LEVEL")
	require.ErrorContains(t, err, "unknown log level")
}

func TestLoad_AcceptsKnownLogLevels(t *testing.T) {
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

func TestLoad_RejectsMaxDelayBelowBaseDelay(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_RETRY_BASE_DELAY", "2s")
	t.Setenv("CORE_RETRY_MAX_DELAY", "1s")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_RETRY_MAX_DELAY")
}

func TestLoad_BreakerDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, uint32(5), cfg.Breaker.FailureThreshold)
	require.Equal(t, 30*time.Second, cfg.Breaker.OpenTimeout)
	require.Equal(t, uint32(1), cfg.Breaker.HalfOpenMaxCalls)
}

func TestLoad_BreakerOverrides(t *testing.T) {
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

func TestLoad_RejectsZeroBreakerLimits(t *testing.T) {
	for _, name := range []string{"CORE_BREAKER_FAILURE_THRESHOLD", "CORE_BREAKER_HALF_OPEN_MAX_CALLS"} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, "0")
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
}

func TestLoad_AppliesHTTPDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.EqualValues(t, 1<<20, cfg.HTTP.MaxBodyBytes)
	require.Equal(t, 30*time.Second, cfg.HTTP.RequestTimeout)
	require.Equal(t, 15*time.Second, cfg.HTTP.EventStreamHeartbeat)
}

func TestLoad_RejectsNonPositiveHTTPSettings(t *testing.T) {
	tests := map[string]string{
		"CORE_HTTP_MAX_BODY_BYTES":         "0",
		"CORE_HTTP_REQUEST_TIMEOUT":        "0s",
		"CORE_HTTP_EVENT_STREAM_HEARTBEAT": "-1s",
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
}

func TestLoad_RequiresVaultMasterKey(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_VAULT_MASTER_KEY", "")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_VAULT_MASTER_KEY")
}
func TestLoad_DefaultsVaultKeyID(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_VAULT_KEY_ID", "")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "v1", cfg.Vault.KeyID)
}
