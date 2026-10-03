package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadRejectsInvalidSchemaConfiguration(t *testing.T) {
	for _, variable := range []string{"CORE_SCHEMA_GENERATION_TIMEOUT", "CORE_SCHEMA_CLEANUP_TIMEOUT"} {
		t.Run(variable, func(t *testing.T) {
			setRequired(t)
			t.Setenv(variable, "0s")
			_, err := Load()
			require.ErrorContains(t, err, "schema timeouts")
		})
	}
}

func TestLoadSchemaDoesNotRequireApplicationDatabase(t *testing.T) {
	unsetEnvironmentVariable(t, "CORE_DATABASE_URL")
	unsetEnvironmentVariable(t, "CORE_REDIS_URL")
	t.Setenv("CORE_ENV_FILE", "")
	configuration, err := LoadSchema()
	require.NoError(t, err)
	require.Equal(t, "postgres:18.6-alpine", configuration.PostgresImage)
	require.Equal(t, 2*time.Minute, configuration.GenerationTimeout)
	require.Equal(t, 15*time.Second, configuration.CleanupTimeout)
}

func TestLoadSchemaReadsFile(t *testing.T) {
	unsetEnvironmentVariable(t, "CORE_SCHEMA_GENERATION_TIMEOUT")
	t.Setenv("CORE_ENV_FILE", writeEnvironmentFile(t, "CORE_SCHEMA_GENERATION_TIMEOUT=10s\n"))
	configuration, err := LoadSchema()
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, configuration.GenerationTimeout)
}
