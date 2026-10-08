package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const (
	migrationStatusCommand        = "status"
	migrationFailureMessage       = "migration failed"
	migrationStatusFailureMessage = "migration status failed"
	migrationsAppliedMessage      = "migrations applied"
	migrationStatusMessage        = "migration status"
)

func (rt *coreRuntime) applyMigrations(ctx context.Context, args []string) error {
	direction := postgres.Up
	if len(args) > 0 {
		if args[0] == migrationStatusCommand {
			return rt.reportMigrationStatus(ctx)
		}
		direction = postgres.Direction(args[0])
	}
	if err := postgres.Migrate(ctx, rt.cfg.DatabaseURL, migrations.FS, direction); err != nil {
		return rt.recordDependencyFailure(ctx, migrationFailureMessage, err, rt.cfg.DatabaseURL)
	}
	rt.log.InfoContext(ctx, migrationsAppliedMessage, logging.FieldDirection, string(direction))
	return nil
}

func (rt *coreRuntime) reportMigrationStatus(ctx context.Context) error {
	statuses, err := postgres.MigrationStatuses(ctx, rt.cfg.DatabaseURL, migrations.FS)
	if err != nil {
		return rt.recordDependencyFailure(ctx, migrationStatusFailureMessage, err, rt.cfg.DatabaseURL)
	}
	for _, status := range statuses {
		rt.log.InfoContext(ctx, migrationStatusMessage, logging.FieldVersion, status.Source.Version, logging.FieldPath, status.Source.Path, logging.FieldState, status.State)
	}
	return nil
}
