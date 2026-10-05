package config

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func composeExample(t *testing.T) map[string]string {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "deploy", "compose", ".env.example"))
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

func TestCompose_ForwardsEveryCoreSetting(t *testing.T) {
	require.Contains(t, composeExample(t), "CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS")
	composeDir := filepath.Join("..", "..", "..", "..", "deploy", "compose")
	command := exec.CommandContext(t.Context(), "docker", "compose", "--env-file", ".env.example", "-f", "compose.infra.yaml", "-f", "compose.observability.yaml", "-f", "compose.yaml", "config", "--format", "json")
	command.Dir = composeDir
	output, err := command.Output()
	require.NoError(t, err, "compose must resolve the example without using local secrets")
	var resolved struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	require.NoError(t, json.Unmarshal(output, &resolved))
	for _, service := range []string{"core", "migrate", "outbox-relay"} {
		require.Equal(t, "100", resolved.Services[service].Environment["CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS"], "%s must forward the IP-only login default", service)
	}
	var assertSettings func(reflect.Type, string)
	assertSettings = func(kind reflect.Type, prefix string) {
		for index := range kind.NumField() {
			field := kind.Field(index)
			if nestedPrefix := field.Tag.Get("envPrefix"); nestedPrefix != "" {
				assertSettings(field.Type, prefix+nestedPrefix)
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("env"), ",")
			if name == "" {
				continue
			}
			for _, service := range []string{"core", "migrate", "outbox-relay"} {
				_, exists := resolved.Services[service].Environment[prefix+name]
				require.True(t, exists, "%s must forward %s", service, prefix+name)
			}
		}
	}
	assertSettings(reflect.TypeFor[Config](), "CORE_")
}
