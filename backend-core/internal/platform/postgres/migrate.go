package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver used by goose
	"github.com/pressly/goose/v3"
)

type Direction string

const (
	Up   Direction = "up"
	Down Direction = "down"
)

func Migrate(ctx context.Context, url string, fsys fs.FS, direction Direction) error {
	provider, db, err := newMigrationProvider(url, fsys)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	switch direction {
	case Up:
		_, err = provider.Up(ctx)
	case Down:
		_, err = provider.Down(ctx)
	default:
		return fmt.Errorf("unknown migration direction %q", direction)
	}
	if err != nil {
		return fmt.Errorf("migrate %s: %w", direction, err)
	}
	return nil
}

func MigrationStatuses(ctx context.Context, url string, fsys fs.FS) ([]*goose.MigrationStatus, error) {
	provider, db, err := newMigrationProvider(url, fsys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	statuses, err := provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration status: %w", err)
	}
	return statuses, nil
}

func newMigrationProvider(url string, fsys fs.FS) (*goose.Provider, *sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, nil, fmt.Errorf("open migration db: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("goose provider: %w", err)
	}
	return provider, db, nil
}
