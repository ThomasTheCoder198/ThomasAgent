package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CORE_AUTH_LOGIN_LIMITER_KEY", "test-login-limiter-key-with-at-least-32-characters")
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
	require.Equal(t, 720*time.Hour, cfg.Conversation.Retention)
	require.Equal(t, time.Hour, cfg.Conversation.RetentionPurgeInterval)
	require.Equal(t, 500, cfg.Conversation.RetentionPurgeBatch)
	require.Equal(t, ":9465", cfg.Worker.Addr)
	require.Equal(t, 30*time.Second, cfg.Worker.DrainTimeout)
	require.Equal(t, 2*time.Minute, cfg.SessionPurgeTimeout)
	require.Equal(t, 10*time.Minute, cfg.ConversationRetentionPurgeTimeout)
}

func TestLoad_RejectsInvalidWorkerTimeouts(t *testing.T) {
	for _, key := range []string{"CORE_WORKER_DRAIN_TIMEOUT", "CORE_SESSION_PURGE_TIMEOUT", "CORE_CONVERSATION_RETENTION_PURGE_TIMEOUT"} {
		t.Run(key, func(t *testing.T) {
			setRequired(t)
			t.Setenv(key, "0s")
			_, err := Load()
			require.ErrorContains(t, err, key)
		})
	}
}

func TestLoad_RejectsInvalidConversationRetention(t *testing.T) {
	for name, value := range map[string]string{
		"CORE_CONVERSATION_RETENTION":                "0s",
		"CORE_CONVERSATION_RETENTION_PURGE_INTERVAL": "0s",
		"CORE_CONVERSATION_RETENTION_PURGE_BATCH":    "0",
	} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
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
	require.Equal(t, uint32(5), cfg.ProviderBreaker.FailureThreshold)
	require.Equal(t, 30*time.Second, cfg.ProviderBreaker.OpenTimeout)
	require.Equal(t, uint32(1), cfg.ProviderBreaker.HalfOpenMaxCalls)
}

func TestLoad_BreakerOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_PROVIDER_BREAKER_FAILURE_THRESHOLD", "7")
	t.Setenv("CORE_PROVIDER_BREAKER_OPEN_TIMEOUT", "45s")
	t.Setenv("CORE_PROVIDER_BREAKER_HALF_OPEN_MAX_CALLS", "3")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, uint32(7), cfg.ProviderBreaker.FailureThreshold)
	require.Equal(t, 45*time.Second, cfg.ProviderBreaker.OpenTimeout)
	require.Equal(t, uint32(3), cfg.ProviderBreaker.HalfOpenMaxCalls)
}

func TestLoad_RejectsZeroBreakerLimits(t *testing.T) {
	for _, name := range []string{"CORE_PROVIDER_BREAKER_FAILURE_THRESHOLD", "CORE_PROVIDER_BREAKER_HALF_OPEN_MAX_CALLS"} {
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
	require.Equal(t, 10*time.Second, cfg.HTTP.EventStreamWriteTimeout)
}

func TestLoad_RejectsNonPositiveHTTPSettings(t *testing.T) {
	tests := map[string]string{
		"CORE_HTTP_MAX_BODY_BYTES":             "0",
		"CORE_HTTP_REQUEST_TIMEOUT":            "0s",
		"CORE_HTTP_EVENT_STREAM_HEARTBEAT":     "-1s",
		"CORE_HTTP_EVENT_STREAM_WRITE_TIMEOUT": "0s",
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
	require.Equal(t, 100, cfg.Auth.LoginIPMaxAttempts)
	require.Greater(t, cfg.Auth.LoginIPMaxAttempts, cfg.Auth.LoginEmailMaxAttempts)
	require.Equal(t, 15*time.Minute, cfg.Auth.LoginWindow)
	require.Equal(t, 12, cfg.Auth.MinPasswordLength)
}

// The service token was removed (ruling R-ST): an old .env that still sets it must not matter.
func TestLoad_IgnoresRemovedServiceToken(t *testing.T) {
	setRequired(t)
	_, err := Load()
	require.NoError(t, err)
	t.Setenv("CORE_SERVICE_TOKEN", "too-short")
	_, err = Load()
	require.NoError(t, err)
}

func TestLoad_RejectsWeakLoginLimiterKeys(t *testing.T) {
	for _, test := range []struct{ name, environment, key, wantError string }{
		{"missing", "dev", "", "CORE_AUTH_LOGIN_LIMITER_KEY"},
		{"short", "dev", "too-short", "at least 32 characters"},
		{"placeholder outside dev", "prod", "change-me-with-a-sufficiently-long-secret-value", "change-me"},
	} {
		t.Run(test.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CORE_ENV", test.environment)
			t.Setenv("CORE_AUTH_LOGIN_LIMITER_KEY", test.key)
			_, err := Load()
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestLoad_AuthOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_AUTH_OWNER_EMAIL", "owner@example.com")
	t.Setenv("CORE_AUTH_OWNER_PASSWORD", "test-owner-password")
	t.Setenv("CORE_AUTH_SESSION_TTL", "2h")
	t.Setenv("CORE_AUTH_COOKIE_SECURE", "true")
	t.Setenv("CORE_AUTH_LOGIN_MAX_ATTEMPTS", "4")
	t.Setenv("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS", "7")
	t.Setenv("CORE_AUTH_LOGIN_WINDOW", "5m")
	t.Setenv("CORE_AUTH_MIN_PASSWORD_LENGTH", "14")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, AuthConfig{OwnerEmail: "owner@example.com", OwnerPassword: "test-owner-password", SessionTTL: 2 * time.Hour, CookieSecure: true, LoginMaxAttempts: 4, LoginWindow: 5 * time.Minute, LoginLimiterKey: "test-login-limiter-key-with-at-least-32-characters", MinPasswordLength: 14, LoginEmailMaxAttempts: 20, LoginIPMaxAttempts: 7, SessionPurgeInterval: time.Hour, SessionTouchInterval: 5 * time.Minute, PasswordHash: PasswordHashConfig{Iterations: 2, MemoryKiB: 65536, MaxConcurrency: 4, Parallelism: 1, SaltLength: 16, KeyLength: 32}}, cfg.Auth)
}

func TestLoad_RejectsWeakPasswordHashParameters(t *testing.T) {
	for name, value := range map[string]string{"CORE_AUTH_PASSWORD_HASH_ITERATIONS": "1", "CORE_AUTH_PASSWORD_HASH_MEMORY_KIB": "32768", "CORE_AUTH_PASSWORD_HASH_MAX_CONCURRENCY": "0"} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
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

func TestLoad_AppTokenDefaultsAndLimits(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "thomas-core", cfg.App.TokenIssuer)
	require.Equal(t, "ragx", cfg.App.TokenAudience)
	require.Equal(t, "dev-1", cfg.App.SigningKeyID)
	require.Equal(t, 60*time.Second, cfg.App.TokenTTL)
	t.Setenv("CORE_APP_TOKEN_TTL", "61s")
	_, err = Load()
	require.ErrorContains(t, err, "CORE_APP_TOKEN_TTL")
	t.Setenv("CORE_APP_TOKEN_TTL", "60s")
	t.Setenv("CORE_APP_SIGNING_KEY", "a")
	t.Setenv("CORE_APP_SIGNING_KEY_FILE", "b")
	_, err = Load()
	require.ErrorContains(t, err, "CORE_APP_SIGNING_KEY")
}

func TestLoad_RagxDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8090", cfg.Ragx.URL)
	require.Equal(t, 1, cfg.Ragx.ProvisionAttempts)
	require.EqualValues(t, 1048576, cfg.Ragx.MaxResponseBytes)
	require.Equal(t, 60*time.Second, cfg.Ragx.IdleConnTimeout)
	t.Setenv("CORE_RAGX_URL", "ftp://ragx")
	_, err = Load()
	require.ErrorContains(t, err, "CORE_RAGX_URL")
}

func TestLoad_AppTokenTTLMustBeWholeSecondsWithinBounds(t *testing.T) {
	for _, ttl := range []string{"0s", "500ms", "1500ms", "61s", "-1s"} {
		t.Run(ttl, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CORE_APP_TOKEN_TTL", ttl)
			_, err := Load()
			require.ErrorContains(t, err, "CORE_APP_TOKEN_TTL")
			require.ErrorContains(t, err, "<= 60s")
		})
	}
	setRequired(t)
	t.Setenv("CORE_APP_TOKEN_TTL", "1s")
	_, err := Load()
	require.NoError(t, err)
}

func TestLoad_RejectsInvalidRagxSettings(t *testing.T) {
	for name, value := range map[string]string{
		"CORE_RAGX_URL": "ragx:8090", "CORE_RAGX_CONNECT_TIMEOUT": "0s", "CORE_RAGX_REQUEST_TIMEOUT": "0s",
		"CORE_RAGX_RESPONSE_HEADER_TIMEOUT": "0s", "CORE_RAGX_IDLE_CONN_TIMEOUT": "0s",
		"CORE_RAGX_PROVISION_ATTEMPTS": "0", "CORE_RAGX_MAX_RESPONSE_BYTES": "0", "CORE_RAGX_MAX_ERROR_BODY_BYTES": "0",
	} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, "CORE_RAGX_")
		})
	}
}

func TestLoad_ProvisionBudgetMustFitTheRequestTimeout(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"two attempts of 15s plus a 10s backoff": {"CORE_RAGX_PROVISION_ATTEMPTS": "2"},
		"one attempt as long as the route":       {"CORE_RAGX_REQUEST_TIMEOUT": "30s"},
	} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			for key, value := range env {
				t.Setenv(key, value)
			}
			_, err := Load()
			require.ErrorContains(t, err, "CORE_RAGX_PROVISION_ATTEMPTS")
		})
	}
	setRequired(t)
	t.Setenv("CORE_RAGX_PROVISION_ATTEMPTS", "2")
	t.Setenv("CORE_RAGX_REQUEST_TIMEOUT", "5s")
	_, err := Load()
	require.NoError(t, err, "2 × 5s + 10s backoff fits the 30s route")
}

func TestLoad_M2aDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 25*time.Second, cfg.Ragx.ToolTimeout)
	require.Equal(t, 30, cfg.Conversation.PageDefaultLimit)
	require.Equal(t, 100, cfg.Conversation.PageMaxLimit)
}

func TestLoad_RejectsDefaultPageAboveMax(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_CONVERSATION_PAGE_DEFAULT_LIMIT", "200")
	t.Setenv("CORE_CONVERSATION_PAGE_MAX_LIMIT", "100")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_CONVERSATION_PAGE_DEFAULT_LIMIT")
}

func TestLoad_RejectsZeroToolTimeout(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_RAGX_TOOL_TIMEOUT", "0s")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_RAGX_TOOL_TIMEOUT")
}

// loadDefaultsForTest loads config.yaml with only the required env set, for default and validate tests.
func loadDefaultsForTest(t *testing.T) Config {
	t.Helper()
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	return cfg
}

func TestConfig_LLMDefaults(t *testing.T) {
	cfg := loadDefaultsForTest(t)
	require.Equal(t, 32, cfg.LLM.GenkitCacheSize)
	require.Equal(t, 10*time.Second, cfg.LLM.DecisionTimeout)
	require.Equal(t, 30*time.Second, cfg.LLM.ProbeTimeout)
}

func TestConfig_LLMRejectsNonPositive(t *testing.T) {
	cfg := loadDefaultsForTest(t)
	cfg.LLM.GenkitCacheSize = 0
	require.ErrorContains(t, cfg.validate(), "CORE_LLM_GENKIT_CACHE_SIZE")
}

func TestLoad_ChatRunDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 8, cfg.Worker.ChatConcurrency)
	require.Equal(t, 5*time.Minute+40*time.Second, cfg.Chat.JobVisibilityTimeout)
	require.Equal(t, 30*time.Second, cfg.Chat.RunLease)
	require.Equal(t, 5*time.Second, cfg.Chat.RunHeartbeatTimeout)
	require.Equal(t, 20000, cfg.Chat.RunStreamMaxEvents)
	require.EqualValues(t, 8388608, cfg.Chat.RunStreamMaxBytes)
	require.Equal(t, 2*time.Minute, cfg.Chat.RunStreamGrace)
	require.Equal(t, 5*time.Second, cfg.Chat.RelayStatusCheck)
	require.Equal(t, 5*time.Minute, cfg.Chat.RunDeadline)
	require.Equal(t, 10*time.Second, cfg.Chat.FinishTimeout)
}

func TestLoad_RejectsInvalidChatRunSettings(t *testing.T) {
	for name, value := range map[string]string{
		"CORE_WORKER_CHAT_CONCURRENCY":     "0",
		"CORE_CHAT_RUN_LEASE":              "0s",
		"CORE_CHAT_RUN_HEARTBEAT_TIMEOUT":  "0s",
		"CORE_CHAT_RUN_STREAM_MAX_EVENTS":  "0",
		"CORE_CHAT_RUN_STREAM_MAX_BYTES":   "0",
		"CORE_CHAT_RUN_STREAM_GRACE":       "0s",
		"CORE_CHAT_RELAY_STATUS_CHECK":     "0s",
		"CORE_CHAT_RUN_DEADLINE":           "0s",
		"CORE_CHAT_FINISH_TIMEOUT":         "0s",
		"CORE_CHAT_JOB_VISIBILITY_TIMEOUT": "0s",
	} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
}

func TestLoad_ChatVisibilityMustExceedRunDeadlinePlusFinishTimeout(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_CHAT_RUN_DEADLINE", "5m")
	t.Setenv("CORE_CHAT_FINISH_TIMEOUT", "10s")
	t.Setenv("CORE_CHAT_JOB_VISIBILITY_TIMEOUT", "5m10s")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_CHAT_JOB_VISIBILITY_TIMEOUT")
	t.Setenv("CORE_CHAT_JOB_VISIBILITY_TIMEOUT", "5m11s")
	_, err = Load()
	require.NoError(t, err)
}

func TestLoad_ChatHeartbeatTimeoutMustFitInsideTheLeaseThird(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_CHAT_RUN_LEASE", "9s")
	t.Setenv("CORE_CHAT_RUN_HEARTBEAT_TIMEOUT", "4s")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_CHAT_RUN_HEARTBEAT_TIMEOUT")
}

func TestConfig_ChatDefaults(t *testing.T) {
	cfg := loadDefaultsForTest(t)
	c := cfg.Chat
	require.Equal(t, 20, c.MaxSteps)
	require.Equal(t, 5*time.Minute, c.RunDeadline)
	require.Equal(t, 20, c.HistoryTurns)
	require.InDelta(t, 0.7, c.CompactionRatio, 1e-9)
	require.Equal(t, 4, c.CompactionKeepTurns)
	require.Equal(t, 4, c.CompactionMaxPages)
	require.InDelta(t, 0.85, c.RouterThreshold, 1e-9)
	require.True(t, c.RouterEnabled)
	require.True(t, c.CitationCheckEnabled)
	require.InDelta(t, 0.5, c.CitationFlagBelow, 1e-9)
	require.Equal(t, 16, c.CitationMaxPairs)
	require.Equal(t, 32000, c.ToolOutputMaxChars)
	require.Equal(t, 60, c.TitleMaxRunes)
	require.Equal(t, 8000, c.MessageMaxRunes)
	require.Equal(t, 5, c.SteerMaxPending)
	require.Equal(t, 2000, c.SteerMaxRunes)
	require.Equal(t, 128000, c.DefaultContextWindow)
	require.Equal(t, 10*time.Second, c.FinishTimeout)
	require.Equal(t, 4, c.MaxParallelTools)
	require.Equal(t, 8, c.MaxToolCallsPerStep)
	require.Equal(t, 4096, c.MaxOutputTokens)
	require.Equal(t, 64, c.TitleMaxOutputTokens)
	require.Equal(t, 1024, c.SummaryMaxOutputTokens)
}

func TestConfig_ChatRejectsOutOfRangeRatios(t *testing.T) {
	cfg := loadDefaultsForTest(t)
	cfg.Chat.CompactionRatio = 1.5
	require.ErrorContains(t, cfg.validate(), "CORE_CHAT_COMPACTION_RATIO")
	cfg = loadDefaultsForTest(t)
	cfg.Chat.MaxParallelTools = 0
	require.ErrorContains(t, cfg.validate(), "CORE_CHAT_")
	cfg = loadDefaultsForTest(t)
	cfg.Chat.CitationFlagBelow = -0.1
	require.ErrorContains(t, cfg.validate(), "CORE_CHAT_CITATION_FLAG_BELOW")
}
