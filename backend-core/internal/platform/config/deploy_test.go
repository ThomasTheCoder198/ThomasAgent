package config

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func composeExample(t *testing.T) map[string]string {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "deploy", "core", ".env.example"))
	require.NoError(t, err)
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		require.True(t, ok, "invalid dotenv line")
		values[key] = value
	}
	require.NoError(t, scanner.Err())
	return values
}

func TestCompose_ExampleConfigurationValid(t *testing.T) {
	setRequired(t)
	for key, value := range composeExample(t) {
		if strings.HasPrefix(key, "CORE_") && value != "" {
			t.Setenv(key, value)
		}
	}
	_, err := Load()
	require.NoError(t, err, "documented example settings must pass config validation")
}

func TestCompose_ExampleBreakerSettingsReachProvider(t *testing.T) {
	for key := range composeExample(t) {
		if !strings.HasPrefix(key, "CORE_BREAKER_") && !strings.HasPrefix(key, "CORE_PROVIDER_BREAKER_") {
			continue
		}
		t.Run(key, func(t *testing.T) {
			setRequired(t)
			switch {
			case strings.HasSuffix(key, "FAILURE_THRESHOLD"):
				t.Setenv(key, "9")
				cfg, err := Load()
				require.NoError(t, err)
				require.EqualValues(t, 9, cfg.ProviderBreaker.FailureThreshold)
			case strings.HasSuffix(key, "OPEN_TIMEOUT"):
				t.Setenv(key, "45s")
				cfg, err := Load()
				require.NoError(t, err)
				require.Equal(t, "45s", cfg.ProviderBreaker.OpenTimeout.String())
			case strings.HasSuffix(key, "HALF_OPEN_MAX_CALLS"):
				t.Setenv(key, "3")
				cfg, err := Load()
				require.NoError(t, err)
				require.EqualValues(t, 3, cfg.ProviderBreaker.HalfOpenMaxCalls)
			}
		})
	}
}

func TestCompose_EnvFileProvidesRuntimeValuesWithoutRepeatingDefaults(t *testing.T) {
	composeDir := filepath.Join("..", "..", "..", "..", "deploy", "core")
	isolatedDir := t.TempDir()
	for _, name := range []string{".env.example", "compose.yaml"} {
		raw, err := os.ReadFile(filepath.Join(composeDir, name))
		require.NoError(t, err)
		if name == ".env.example" {
			name = ".env"
		}
		require.NoError(t, os.WriteFile(filepath.Join(isolatedDir, name), raw, 0o600))
	}
	command := exec.CommandContext(t.Context(), "docker", "compose", "-f", "compose.yaml", "config", "--format", "json")
	command.Dir = isolatedDir
	output, err := command.Output()
	require.NoError(t, err, "compose must resolve the example without using local secrets")
	var resolved struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	require.NoError(t, json.Unmarshal(output, &resolved))
	for _, service := range []string{"core", "migrate", "outbox-relay"} {
		environment := resolved.Services[service].Environment
		require.NotContains(t, environment, "CORE_SERVICE_TOKEN", "the service token was removed (R-ST)")
		require.Contains(t, environment["CORE_DATABASE_URL"], "@postgres:5432/")
		require.Equal(t, "redis://redis:6379/0", environment["CORE_REDIS_URL"])
		require.NotContains(t, environment, "CORE_AUTH_LOGIN_MAX_ATTEMPTS", "defaults belong in config.yaml")
	}
}

func TestCompose_CoreStackHasNoLegacyObservability(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", "core", "compose.yaml"))
	require.NoError(t, err)
	for _, legacy := range []string{"otel-collector", "loki", "langfuse", "clickhouse", "web:"} {
		require.NotContains(t, string(raw), legacy)
	}
	for _, service := range []string{"core:", "migrate:", "outbox-relay:", "postgres:", "redis:", "prometheus:", "grafana:", "victorialogs:"} {
		require.Contains(t, string(raw), service)
	}
}

func TestCompose_CoreExampleDoesNotRequireLocalSecrets(t *testing.T) {
	composeDir := filepath.Join("..", "..", "..", "..", "deploy", "core")
	isolatedDir := t.TempDir()
	for _, name := range []string{".env.example", "compose.yaml"} {
		raw, err := os.ReadFile(filepath.Join(composeDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(isolatedDir, name), raw, 0o600))
	}
	command := exec.CommandContext(t.Context(), "docker", "compose", "--env-file", ".env.example", "-f", "compose.yaml", "config", "--quiet")
	command.Dir = isolatedDir
	output, err := command.CombinedOutput()
	require.NoError(t, err, "example-only compose must resolve on a clean checkout: %s", output)
}
