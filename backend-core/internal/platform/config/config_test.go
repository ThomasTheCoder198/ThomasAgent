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
}

func TestLoadRejectsMaxDelayBelowBaseDelay(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_RETRY_BASE_DELAY", "2s")
	t.Setenv("CORE_RETRY_MAX_DELAY", "1s")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_RETRY_MAX_DELAY")
}
