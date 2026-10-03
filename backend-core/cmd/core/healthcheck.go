package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const localHealthBaseURL = "http://127.0.0.1"

func checkHTTPHealth(ctx context.Context, addr string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, localHealthBaseURL+addr+httpx.LivenessPath, nil)
	if err != nil {
		return fmt.Errorf("create healthcheck request: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz status %d", res.StatusCode)
	}
	return nil
}
