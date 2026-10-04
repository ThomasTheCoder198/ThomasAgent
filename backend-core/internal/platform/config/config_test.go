package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CORE_SERVICE_TOKEN", "test-token")
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

func TestLoad_AuthDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 720*time.Hour, cfg.Auth.SessionTTL)
	require.False(t, cfg.Auth.CookieSecure)
	require.Equal(t, 10, cfg.Auth.LoginMaxAttempts)
	require.Equal(t, 15*time.Minute, cfg.Auth.LoginWindow)
	require.Equal(t, 12, cfg.Auth.MinPasswordLength)
	require.Equal(t, "test-token", cfg.ServiceToken)
}
func TestLoad_RequiresServiceToken(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_SERVICE_TOKEN", "")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_SERVICE_TOKEN")
}
func TestLoad_AuthOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_AUTH_OWNER_EMAIL", "owner@example.com")
	t.Setenv("CORE_AUTH_OWNER_PASSWORD", "test-owner-password")
	t.Setenv("CORE_AUTH_SESSION_TTL", "2h")
	t.Setenv("CORE_AUTH_COOKIE_SECURE", "true")
	t.Setenv("CORE_AUTH_LOGIN_MAX_ATTEMPTS", "4")
	t.Setenv("CORE_AUTH_LOGIN_WINDOW", "5m")
	t.Setenv("CORE_AUTH_MIN_PASSWORD_LENGTH", "14")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, AuthConfig{OwnerEmail: "owner@example.com", OwnerPassword: "test-owner-password", SessionTTL: 2 * time.Hour, CookieSecure: true, LoginMaxAttempts: 4, LoginWindow: 5 * time.Minute, MinPasswordLength: 14}, cfg.Auth)
}

func TestLoad_RejectsNonPositiveAuthSettings(t *testing.T) {
	for _, name := range []string{"CORE_AUTH_SESSION_TTL", "CORE_AUTH_LOGIN_WINDOW", "CORE_AUTH_LOGIN_MAX_ATTEMPTS", "CORE_AUTH_MIN_PASSWORD_LENGTH"} {
		for _, value := range []string{"0", "-1"} {
			t.Run(name+"/"+value, func(t *testing.T) {
				setRequired(t)
				if name == "CORE_AUTH_SESSION_TTL" || name == "CORE_AUTH_LOGIN_WINDOW" {
					value += "s"
				}
				t.Setenv(name, value)
				_, err := Load()
				require.ErrorContains(t, err, name)
			})
		}
	}
}

func TestLoad_ProviderDefaultsAndPrivateAllowlist(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 20*time.Second, cfg.ProviderHTTPTimeout)
	require.EqualValues(t, 8388608, cfg.ProviderMaxResponseBytes)
	require.Empty(t, cfg.ProviderPrivateAllowlist)
	require.Equal(t, BreakerConfig{FailureThreshold: 5, OpenTimeout: 30 * time.Second, HalfOpenMaxCalls: 1}, cfg.ProviderBreaker)
	t.Setenv("CORE_PROVIDER_PRIVATE_ALLOWLIST", "localhost:11434,[::1]:8080")
	cfg, err = Load()
	require.NoError(t, err)
	require.Equal(t, []string{"localhost:11434", "[::1]:8080"}, cfg.ProviderPrivateAllowlist)
}

func TestLoad_RejectsInvalidProviderSettings(t *testing.T) {
	for name, value := range map[string]string{"CORE_PROVIDER_HTTP_TIMEOUT": "0s", "CORE_PROVIDER_MAX_RESPONSE_BYTES": "0", "CORE_PROVIDER_BREAKER_FAILURE_THRESHOLD": "0", "CORE_PROVIDER_BREAKER_OPEN_TIMEOUT": "0s", "CORE_PROVIDER_BREAKER_HALF_OPEN_MAX_CALLS": "0", "CORE_PROVIDER_PRIVATE_ALLOWLIST": "https://localhost:11434"} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
}
