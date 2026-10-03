package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignalEndpointNormalizesTrailingSlashes(t *testing.T) {
	for _, base := range []string{"http://collector:4318", "http://collector:4318/", "http://collector:4318///"} {
		t.Run(base, func(t *testing.T) {
			require.Equal(t, "http://collector:4318/v1/traces", signalEndpoint(base, tracesPath))
			require.Equal(t, "http://collector:4318/v1/logs", signalEndpoint(base, logsPath))
		})
	}
}
