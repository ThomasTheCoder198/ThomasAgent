package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_MetricsAndCallLogDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":9464", cfg.Metrics.Addr)
	require.Equal(t, []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}, cfg.Metrics.HTTPBuckets)
	require.Equal(t, []float64{0.25, 0.5, 1, 2, 4, 8, 16, 32, 64}, cfg.Metrics.LLMBuckets)
}

func TestLoad_RejectsUnsortedBuckets(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_METRICS_HTTP_BUCKETS", "1,0.5")
	_, err := Load()
	require.ErrorContains(t, err, "CORE_METRICS_HTTP_BUCKETS")
}

func TestLoad_RejectsInvalidMetricsBuckets(t *testing.T) {
	for _, key := range []string{"CORE_METRICS_HTTP_BUCKETS", "CORE_METRICS_LLM_BUCKETS"} {
		for _, value := range []string{"0", "-1", "1,1", "NaN", "+Inf", "1,+Inf"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				setRequired(t)
				t.Setenv(key, value)
				_, err := Load()
				require.ErrorContains(t, err, key)
			})
		}
	}
}

func TestLoad_MetricsCanBeDisabled(t *testing.T) {
	setRequired(t)
	t.Setenv("CORE_METRICS_ADDR", "")
	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.Metrics.Addr)
}

func TestLoad_LLMLogDefaultsAndValidation(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.LLMLog.Content)
	require.Equal(t, 16384, cfg.LLMLog.ContentMaxBytes)
	t.Setenv("CORE_LLM_LOG_CONTENT_MAX_BYTES", "0")
	_, err = Load()
	require.ErrorContains(t, err, "CORE_LLM_LOG_CONTENT_MAX_BYTES")
}
