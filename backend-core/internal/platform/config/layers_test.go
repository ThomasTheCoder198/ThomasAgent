package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ConfigYAMLThenDotenvThenProcessEnvironment(t *testing.T) {
	setRequired(t)
	file := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(file, []byte("CORE_AUTH_LOGIN_MAX_ATTEMPTS=7\nCORE_LOG_LEVEL=debug\n"), 0o600))
	t.Setenv(envFileEnv, file)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 7, cfg.Auth.LoginMaxAttempts)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, 100, cfg.Auth.LoginIPMaxAttempts)

	t.Setenv("CORE_AUTH_LOGIN_MAX_ATTEMPTS", "3")
	cfg, err = Load()
	require.NoError(t, err)
	require.Equal(t, 3, cfg.Auth.LoginMaxAttempts)
}

func TestLoad_ExplicitDotenvPathMustExist(t *testing.T) {
	setRequired(t)
	t.Setenv(envFileEnv, filepath.Join(t.TempDir(), "missing.env"))
	_, err := Load()
	require.ErrorContains(t, err, "load dotenv")
}
