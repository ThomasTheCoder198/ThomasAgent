package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ValidatesLogsEndpoint(t *testing.T) {
	for value, valid := range map[string]bool{
		"": true,
		"http://victorialogs:9428/insert/opentelemetry/v1/logs": true,
		"http://otel-collector:4318/v1/logs":                    true,
		"http://otel-collector:4318":                            false,
		"http://otel-collector:4318/":                           false,
		"otel-collector:4318/v1/logs":                           false,
		"ftp://collector/v1/logs":                               false,
	} {
		t.Run(value, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CORE_LOGS_OTLP_ENDPOINT", value)
			cfg, err := Load()
			if valid {
				require.NoError(t, err)
				require.Equal(t, value, cfg.LogsOTLPEndpoint)
				return
			}
			require.ErrorContains(t, err, "CORE_LOGS_OTLP_ENDPOINT")
		})
	}
}
