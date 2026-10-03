package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const (
	exitOK             = 0
	exitFailure        = 1
	minArgs            = 2
	serveCommand       = "serve"
	migrateCommand     = "migrate"
	relayCommand       = "relay"
	healthcheckCommand = "healthcheck"
)

const usage = "usage: core <serve|migrate|relay|healthcheck> [args]"

func runProcess() int {
	if len(os.Args) < minArgs {
		fmt.Fprintln(os.Stderr, usage)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := executeCommand(ctx, os.Args[1], os.Args[minArgs:]); err != nil {
		var logged loggedError
		if !errors.As(err, &logged) {
			fmt.Fprintln(os.Stderr, "core:", err)
		}
		return exitFailure
	}
	return exitOK
}

func executeCommand(ctx context.Context, command string, args []string) error {
	switch command {
	case serveCommand, migrateCommand, relayCommand, healthcheckCommand:
	default:
		return fmt.Errorf("unknown command %q; %s", command, usage)
	}
	a, err := newApplication(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.shutdownTelemetry() }()
	switch command {
	case healthcheckCommand:
		return checkHTTPHealth(ctx, a.cfg.HTTPAddr)
	case serveCommand:
		return a.serveHTTP(ctx)
	case relayCommand:
		return a.runOutboxRelay(ctx)
	case migrateCommand:
		return a.applyMigrations(ctx, args)
	default:
		return fmt.Errorf("unknown command %q; %s", command, usage)
	}
}
