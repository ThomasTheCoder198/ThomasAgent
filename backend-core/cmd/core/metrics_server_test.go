package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
)

func metricsApp(t *testing.T, addr string, output *bytes.Buffer) *coreRuntime {
	t.Helper()
	log, err := logging.NewLogger(output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	return &coreRuntime{
		cfg:     config.Config{Metrics: config.MetricsConfig{Addr: addr}, ShutdownTimeout: time.Second, ReadHeaderTimeout: time.Second},
		log:     log,
		metrics: metrics.New([]float64{0.1, 1}, []float64{1, 10}),
	}
}

func TestServeMetrics_ServesOpenMetricsUntilCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	var output bytes.Buffer
	app := metricsApp(t, addr, &output)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { app.serveMetrics(ctx); close(done) }()
	require.Eventually(t, func() bool {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/metrics", nil)
		require.NoError(t, reqErr)
		res, getErr := http.DefaultClient.Do(req)
		if getErr != nil {
			return false
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode == http.StatusOK && bytes.Contains(body, []byte("go_goroutines"))
	}, 5*time.Second, 20*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("metrics server did not stop on cancellation")
	}
}

func TestServeMetrics_BindFailureIsLoggedNotFatal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	var output bytes.Buffer
	app := metricsApp(t, listener.Addr().String(), &output)
	app.serveMetrics(t.Context()) // returns instead of exiting the process
	require.Contains(t, output.String(), "metrics server stopped")
}

func TestServeMetrics_EmptyAddressDisables(t *testing.T) {
	var output bytes.Buffer
	app := metricsApp(t, "", &output)
	app.serveMetrics(t.Context())
	require.Empty(t, output.String())
}
