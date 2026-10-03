package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func unsetEnvironmentVariable(t *testing.T, key string) {
	t.Helper()
	previous, present := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if present {
			require.NoError(t, os.Setenv(key, previous))
		} else {
			require.NoError(t, os.Unsetenv(key))
		}
	})
}

func writeEnvironmentFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestLoadReadsEnvironmentFile(t *testing.T) {
	unsetEnvironmentVariable(t, "CORE_DATABASE_URL")
	unsetEnvironmentVariable(t, "CORE_REDIS_URL")
	unsetEnvironmentVariable(t, "CORE_LOG_LEVEL")
	t.Setenv("CORE_ENV_FILE", writeEnvironmentFile(t, "CORE_DATABASE_URL=postgres://file:pass@localhost/core\nCORE_REDIS_URL=redis://localhost:6379/0\nCORE_LOG_LEVEL=debug\n"))
	configuration, err := Load()
	require.NoError(t, err)
	require.Equal(t, "postgres://file:pass@localhost/core", configuration.DatabaseURL)
	require.Equal(t, "debug", configuration.LogLevel)
	_, mutated := os.LookupEnv("CORE_DATABASE_URL")
	require.False(t, mutated)
}

func TestProcessEnvironmentOverridesFile(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_ENV_FILE", writeEnvironmentFile(t, "CORE_DATABASE_URL=postgres://file:pass@localhost/core\nCORE_REDIS_URL=redis://localhost:6379/0\nCORE_LOG_LEVEL=debug\n"))
	t.Setenv("CORE_LOG_LEVEL", "warn")
	configuration, err := Load()
	require.NoError(t, err)
	require.Equal(t, "postgres://u:p@localhost:5432/thomas", configuration.DatabaseURL)
	require.Equal(t, "warn", configuration.LogLevel)
}

func TestLoadReadsDefaultEnvironmentFile(t *testing.T) {
	unsetEnvironmentVariable(t, "CORE_ENV_FILE")
	unsetEnvironmentVariable(t, "CORE_DATABASE_URL")
	unsetEnvironmentVariable(t, "CORE_REDIS_URL")
	environmentFile := writeEnvironmentFile(t, "CORE_DATABASE_URL=postgres://file:pass@localhost/core\nCORE_REDIS_URL=redis://localhost:6379/0\n")
	t.Chdir(filepath.Dir(environmentFile))
	configuration, err := Load()
	require.NoError(t, err)
	require.Equal(t, "postgres://file:pass@localhost/core", configuration.DatabaseURL)
}

func TestLoadRejectsMissingExplicitEnvironmentFile(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	_, err := Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "CORE_ENV_FILE")
}

func TestLoadRejectsMalformedEnvironmentFileWithoutSecrets(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_ENV_FILE", writeEnvironmentFile(t, "BAD@KEY=secret-password\n"))
	_, err := Load()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-password")
}

func TestEmptyProcessVariableDoesNotFallBackToFile(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_ENV_FILE", writeEnvironmentFile(t, "CORE_DATABASE_URL=postgres://file:pass@localhost/core\n"))
	t.Setenv("CORE_DATABASE_URL", "")
	_, err := Load()
	require.Error(t, err)
}
