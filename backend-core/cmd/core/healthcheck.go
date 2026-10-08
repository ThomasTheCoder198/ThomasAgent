package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const (
	localHealthHost    = "127.0.0.1"
	localHealthScheme  = "http://"
	healthcheckTimeout = 3 * time.Second
)

func checkHTTPHealth(ctx context.Context, addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("healthcheck address: %w", err)
	}
	url := localHealthScheme + net.JoinHostPort(localHealthHost, port) + httpx.LivenessPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create healthcheck request: %w", err)
	}
	client := &http.Client{Timeout: healthcheckTimeout}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz status %d", res.StatusCode)
	}
	return nil
}
