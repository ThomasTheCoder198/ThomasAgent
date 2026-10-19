package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	apperrors "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	exitOK             = 0
	exitFailure        = 1
	minArgsWithCommand = 2
	serveCommand       = "serve"
	migrateCommand     = "migrate"
	outboxRelayCommand = "outbox-relay"
	workerCommand      = "worker"
	healthcheckCommand = "healthcheck"
	apiTarget          = "api"
	targetFlag         = "--target"
	targetFlagArgs     = 2
)

const usageMessage = "usage: core <serve|migrate|worker|outbox-relay|healthcheck> [args]"

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
	return executeCommandWith(ctx, command, args, newCoreRuntime)
}

// executeCommandWith takes the runtime factory so tests can run a command against their own dependencies.
func executeCommandWith(ctx context.Context, command string, args []string, newRuntime func(context.Context) (*coreRuntime, error)) error {
	var run func(*coreRuntime) error
	switch command {
	case healthcheckCommand:
		run = func(rt *coreRuntime) error { return runHealthcheck(ctx, rt, args) }
	case serveCommand:
		run = func(rt *coreRuntime) error { return rt.serveHTTP(ctx) }
	case outboxRelayCommand:
		run = func(rt *coreRuntime) error {
			rt.log.WarnContext(ctx, "outbox-relay command is deprecated; use worker")
			return rt.runWorker(ctx)
		}
	case workerCommand:
		run = func(rt *coreRuntime) error { return rt.runWorker(ctx) }
	case migrateCommand:
		run = func(rt *coreRuntime) error { return rt.applyMigrations(ctx, args) }
	default:
		return fmt.Errorf("unknown command %q; %s", command, usageMessage)
	}
	rt, err := newRuntime(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rt.shutdownTelemetry() }()
	return run(rt)
}

func runHealthcheck(ctx context.Context, rt *coreRuntime, args []string) error {
	if len(args) == 0 {
		return checkHTTPHealth(ctx, rt.cfg.HTTPAddr)
	}
	if len(args) != targetFlagArgs || args[0] != targetFlag {
		return apperrors.ErrHealthcheckTargetInvalid
	}
	switch args[1] {
	case apiTarget:
		return checkHTTPHealth(ctx, rt.cfg.HTTPAddr)
	case workerCommand:
		return checkHTTPHealth(ctx, rt.cfg.Worker.Addr)
	default:
		return apperrors.ErrHealthcheckTargetInvalid
	}
}
