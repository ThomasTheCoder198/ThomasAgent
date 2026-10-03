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
	Up     Direction = "up"
	Down   Direction = "down"
	Status Direction = "status"
)

func Migrate(ctx context.Context, url string, fsys fs.FS, dir Direction) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open migration db: %w", err)
	}
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	switch dir {
	case Up:
		_, err = provider.Up(ctx)
	case Down:
		_, err = provider.Down(ctx)
	case Status:
		_, err = provider.Status(ctx)
	default:
		return fmt.Errorf("unknown migration direction %q", dir)
	}
	if err != nil {
		return fmt.Errorf("migrate %s: %w", dir, err)
	}
	return nil
}
