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
	minArgsWithCommand = 2
	serveCommand       = "serve"
	migrateCommand     = "migrate"
	outboxRelayCommand = "outbox-relay"
	healthcheckCommand = "healthcheck"
)

const usageMessage = "usage: core <serve|migrate|outbox-relay|healthcheck> [args]"

func runProcess() int {
	if len(os.Args) < minArgsWithCommand {
		fmt.Fprintln(os.Stderr, usageMessage)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := executeCommand(ctx, os.Args[1], os.Args[minArgsWithCommand:]); err != nil {
		var logged alreadyLoggedError
		if !errors.As(err, &logged) {
			fmt.Fprintln(os.Stderr, "core:", err)
		}
		return exitFailure
	}
	return exitOK
}

func executeCommand(ctx context.Context, command string, args []string) error {
	switch command {
	case serveCommand, migrateCommand, outboxRelayCommand, healthcheckCommand:
	default:
		return fmt.Errorf("unknown command %q; %s", command, usageMessage)
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
	case outboxRelayCommand:
		return a.runOutboxRelay(ctx)
	case migrateCommand:
		return a.applyMigrations(ctx, args)
	default:
		return fmt.Errorf("unknown command %q; %s", command, usageMessage)
	}
}
