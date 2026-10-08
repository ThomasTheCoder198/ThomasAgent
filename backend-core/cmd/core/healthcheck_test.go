package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthcheck_ProbesLoopbackForEveryListenAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/healthz", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	for _, addr := range []string{
		":" + port,
		net.JoinHostPort("0.0.0.0", port),
		net.JoinHostPort("127.0.0.1", port),
	} {
		t.Run(addr, func(t *testing.T) { require.NoError(t, checkHTTPHealth(t.Context(), addr)) })
	}
}
