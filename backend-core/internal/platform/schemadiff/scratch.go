package schemadiff

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"ariga.io/atlas/sql/migrate"
	atlaspostgres "ariga.io/atlas/sql/postgres"
	"ariga.io/atlas/sql/schema"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	corepostgres "github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

const (
	scratchDatabase       = "history"
	desiredDatabase       = "desired"
	scratchUsername       = "migrationgen"
	scratchPassword       = "migrationgen"
	postgresDriver        = "pgx"
	applicationSchema     = "public"
	gooseVersionTable     = "goose_db_version"
	createDesiredDatabase = "CREATE DATABASE desired"
)

type scratchDatabasePair struct {
	container                    *postgres.PostgresContainer
	historyURL                   string
	history, desired             *sql.DB
	historyDriver, desiredDriver migrate.Driver
	historySchema, desiredSchema *schema.Schema
	tracer                       trace.Tracer
	historyFiles                 fstest.MapFS
}

func startScratch(ctx context.Context, settings config.SchemaConfig, tracer trace.Tracer) (*scratchDatabasePair, error) {
	ctx, span := tracer.Start(ctx, "schema.scratch.start")
	defer span.End()
	container, err := postgres.Run(ctx, settings.PostgresImage, postgres.WithDatabase(scratchDatabase),
		postgres.WithUsername(scratchUsername), postgres.WithPassword(scratchPassword), postgres.BasicWaitStrategies())
	if err != nil {
		return nil, fmt.Errorf("start disposable PostgreSQL: %w", err)
	}
	scratch := &scratchDatabasePair{container: container, tracer: tracer}
	if err := scratch.open(ctx); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settings.CleanupTimeout)
		defer cancel()
		return nil, stderrors.Join(err, scratch.close(cleanupCtx))
	}
	return scratch, nil
}

func (s *scratchDatabasePair) open(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "schema.scratch.open")
	defer span.End()
	dsn, err := s.container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("scratch connection: %w", err)
	}
	s.historyURL = dsn
	s.history, err = sql.Open(postgresDriver, dsn)
	if err != nil {
		return fmt.Errorf("open history database: %w", err)
	}
	if _, err := s.history.ExecContext(ctx, createDesiredDatabase); err != nil {
		return fmt.Errorf("create desired database: %w", err)
	}
	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("scratch URL: %w", err)
	}
	parsedURL.Path = desiredDatabase
	s.desired, err = sql.Open(postgresDriver, parsedURL.String())
	if err != nil {
		return fmt.Errorf("open desired database: %w", err)
	}
	s.historyDriver, err = atlaspostgres.Open(s.history)
	if err != nil {
		return fmt.Errorf("inspect history driver: %w", err)
	}
	s.desiredDriver, err = atlaspostgres.Open(s.desired)
	if err != nil {
		return fmt.Errorf("inspect desired driver: %w", err)
	}
	return nil
}

func (s *scratchDatabasePair) close(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "schema.scratch.close")
	defer span.End()
	var errs []error
	if s.history != nil {
		errs = append(errs, s.history.Close())
	}
	if s.desired != nil {
		errs = append(errs, s.desired.Close())
	}
	errs = append(errs, s.container.Terminate(ctx))
	return stderrors.Join(errs...)
}

func (s *scratchDatabasePair) buildPlan(ctx context.Context, request Request, ddl []byte) (*migrate.Plan, error) {
	ctx, span := s.tracer.Start(ctx, "schema.plan")
	defer span.End()
	var err error
	s.historyFiles, err = loadMigrationHistory(request.MigrationsDir)
	if err != nil {
		return nil, err
	}
	if err := corepostgres.Migrate(ctx, s.historyURL, s.historyFiles, corepostgres.Up); err != nil {
		return nil, fmt.Errorf("replay migration history: %w", err)
	}
	if _, err := s.desired.ExecContext(ctx, string(ddl)); err != nil {
		return nil, errors.ErrValidationFailed.WithCause(fmt.Errorf("execute desired schema: %w", err))
	}
	if err := s.inspectSchemas(ctx); err != nil {
		return nil, err
	}
	changes, err := s.historyDriver.SchemaDiff(s.historySchema, s.desiredSchema)
	if err != nil {
		return nil, fmt.Errorf("diff schemas: %w", err)
	}
	plan, err := s.historyDriver.PlanChanges(ctx, request.MigrationName, changes)
	if err != nil {
		return nil, fmt.Errorf("plan migration: %w", err)
	}
	if !plan.Reversible || !plan.Transactional {
		return nil, fmt.Errorf("migration must be reversible and transactional; write this migration manually")
	}
	return plan, nil
}

func (s *scratchDatabasePair) inspectSchemas(ctx context.Context) error {
	if err := rejectUnsupportedObjects(ctx, s.history, gooseVersionTable); err != nil {
		return fmt.Errorf("migration history: %w", err)
	}
	if err := rejectUnsupportedObjects(ctx, s.desired, ""); err != nil {
		return fmt.Errorf("desired schema: %w", err)
	}
	var err error
	s.historySchema, err = s.historyDriver.InspectSchema(ctx, applicationSchema, &schema.InspectOptions{Exclude: []string{gooseVersionTable}})
	if err != nil {
		return fmt.Errorf("inspect history schema: %w", err)
	}
	s.desiredSchema, err = s.desiredDriver.InspectSchema(ctx, applicationSchema, nil)
	if err != nil {
		return fmt.Errorf("inspect desired schema: %w", err)
	}
	return nil
}

func (s *scratchDatabasePair) validateMigration(ctx context.Context, filename string, contents []byte) error {
	ctx, span := s.tracer.Start(ctx, "schema.validate")
	defer span.End()
	// Goose needs the previous version in its source filesystem when rolling back the new migration.
	migrationFS := s.historyFiles
	migrationFS[filename] = &fstest.MapFile{Data: contents}
	if err := corepostgres.Migrate(ctx, s.historyURL, migrationFS, corepostgres.Up); err != nil {
		return fmt.Errorf("validate migration Up: %w", err)
	}
	if err := s.assertHistoryMatches(ctx, s.desiredSchema); err != nil {
		return fmt.Errorf("validate migration Up: %w", err)
	}
	if err := corepostgres.Migrate(ctx, s.historyURL, migrationFS, corepostgres.Down); err != nil {
		return fmt.Errorf("validate migration Down: %w", err)
	}
	if err := s.assertHistoryMatches(ctx, s.historySchema); err != nil {
		return fmt.Errorf("validate migration Down: %w", err)
	}
	return nil
}

func (s *scratchDatabasePair) assertHistoryMatches(ctx context.Context, expected *schema.Schema) error {
	actual, err := s.historyDriver.InspectSchema(ctx, applicationSchema, &schema.InspectOptions{Exclude: []string{gooseVersionTable}})
	if err != nil {
		return fmt.Errorf("inspect migrated schema: %w", err)
	}
	changes, err := s.historyDriver.SchemaDiff(actual, expected)
	if err != nil {
		return fmt.Errorf("compare migrated schema: %w", err)
	}
	if len(changes) != 0 {
		return fmt.Errorf("migration does not restore the expected schema")
	}
	return nil
}

func loadMigrationHistory(directory string) (fstest.MapFS, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migration history: %w", err)
	}
	files := make(fstest.MapFS)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), migrationExtension) {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		files[entry.Name()] = &fstest.MapFile{Data: contents}
	}
	return files, nil
}
