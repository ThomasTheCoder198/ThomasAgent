package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_LoginProxyTrustDefaultsToDisabled(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.Auth.TrustedProxyHeader)
}

func TestLoad_RejectsInvalidHardeningSettings(t *testing.T) {
	for name, value := range map[string]string{
		"CORE_AUTH_TRUSTED_PROXY_CIDRS":       "not-a-network",
		"CORE_AUTH_LOGIN_EMAIL_MAX_ATTEMPTS":  "0",
		"CORE_AUTH_SESSION_TOUCH_INTERVAL":    "0s",
		"CORE_AUTH_PASSWORD_HASH_PARALLELISM": "0",
		"CORE_AUTH_PASSWORD_HASH_SALT_LENGTH": "7",
		"CORE_AUTH_PASSWORD_HASH_KEY_LENGTH":  "15",
		"CORE_PROVIDER_MAX_REMOTE_MODELS":     "0",
		"CORE_PROVIDER_MAX_BREAKERS":          "0",
	} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, value)
			_, err := Load()
			require.ErrorContains(t, err, name)
		})
	}
}

func TestLoad_UsesOnlyProviderBreakerConfiguration(t *testing.T) {
	setRequired(t)
	_, obsolete := reflect.TypeFor[Config]().FieldByName("Breaker")
	require.False(t, obsolete, "unused second breaker configuration must not silently accept deployment tuning")
}

func TestLoad_RejectsInvalidLoginIPLimit(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		t.Run(value, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS", value)
			_, err := Load()
			require.ErrorContains(t, err, "CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS")
		})
	}
}
