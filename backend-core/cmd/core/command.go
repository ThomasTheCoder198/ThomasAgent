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
	var run func(*coreRuntime) error
	switch command {
	case healthcheckCommand:
		run = func(rt *coreRuntime) error { return checkHTTPHealth(ctx, rt.cfg.HTTPAddr) }
	case serveCommand:
		run = func(rt *coreRuntime) error { return rt.serveHTTP(ctx) }
	case outboxRelayCommand:
		run = func(rt *coreRuntime) error { return rt.runOutboxRelay(ctx) }
	case migrateCommand:
		run = func(rt *coreRuntime) error { return rt.applyMigrations(ctx, args) }
	default:
		return fmt.Errorf("unknown command %q; %s", command, usageMessage)
	}
	rt, err := newCoreRuntime(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rt.shutdownTelemetry() }()
	return run(rt)
}
