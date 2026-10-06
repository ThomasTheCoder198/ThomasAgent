package telemetry

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const exportWait = 5 * time.Second

func TestSetup_EmptyEndpointDisablesExport(t *testing.T) {
	tel, err := Setup(t.Context(), "core-test", "test", "")
	require.NoError(t, err)
	require.Nil(t, tel.LogHandler)
	require.NoError(t, tel.Shutdown(t.Context()))
}

func TestSetup_PostsLogsToTheFullConfiguredURL(t *testing.T) {
	paths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case paths <- r.URL.Path:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	tel, err := Setup(t.Context(), "core-test", "test", server.URL+"/insert/opentelemetry/v1/logs")
	require.NoError(t, err)
	require.NotNil(t, tel.LogHandler)
	slog.New(tel.LogHandler).InfoContext(t.Context(), "exported")
	_ = tel.Shutdown(t.Context()) // flushes the batch; an empty 200 body may be reported as a decode error
	select {
	case path := <-paths:
		require.Equal(t, "/insert/opentelemetry/v1/logs", path, "the configured URL is used as is, no /v1/logs suffix")
	case <-time.After(exportWait):
		t.Fatal("no log export reached the configured URL")
	}
}
