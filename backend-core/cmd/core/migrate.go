package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const migrationStatusCommand = "status"

func (a *application) applyMigrations(ctx context.Context, args []string) error {
	direction := postgres.Up
	if len(args) > 0 {
		if args[0] == migrationStatusCommand {
			return a.reportMigrationStatus(ctx)
		}
		direction = postgres.Direction(args[0])
	}
	if err := postgres.Migrate(ctx, a.cfg.DatabaseURL, migrations.FS, direction); err != nil {
		return a.recordDependencyFailure(ctx, "migration failed", err, a.cfg.DatabaseURL)
	}
	a.log.InfoContext(ctx, "migrations applied", "direction", string(direction))
	return nil
}

func (a *application) reportMigrationStatus(ctx context.Context) error {
	statuses, err := postgres.MigrationStatuses(ctx, a.cfg.DatabaseURL, migrations.FS)
	if err != nil {
		return a.recordDependencyFailure(ctx, "migration status failed", err, a.cfg.DatabaseURL)
	}
	for _, status := range statuses {
		a.log.InfoContext(ctx, "migration status", "version", status.Source.Version, "path", status.Source.Path, "state", status.State)
	}
	return nil
}
