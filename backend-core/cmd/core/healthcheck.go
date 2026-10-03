package main

import (
	"context"
	"fmt"
	"net/http"
)

const healthcheckEndpoint = "http://127.0.0.1"
const healthcheckPath = "/healthz"

func checkHTTPHealth(ctx context.Context, addr string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthcheckEndpoint+addr+healthcheckPath, nil)
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
