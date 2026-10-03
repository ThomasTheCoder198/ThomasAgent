package schemadiff

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ariga.io/atlas/sql/sqltool"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

type Request struct{ MigrationName, SchemaPath, MigrationsDir string }
type Result struct{ Path string }
type Generator struct {
	settings config.SchemaConfig
	now      func() time.Time
	tracer   trace.Tracer
}

func New(settings config.SchemaConfig, now func() time.Time, tracer trace.Tracer) *Generator {
	return &Generator{settings: settings, now: now, tracer: tracer}
}

func (g *Generator) Generate(ctx context.Context, request Request) (result Result, retErr error) {
	ctx, span := g.tracer.Start(ctx, "schema.generate")
	defer span.End()
	defer func() {
		if retErr != nil {
			retErr = errors.From(retErr)
			span.RecordError(retErr)
			span.SetStatus(codes.Error, "migration generation failed")
		}
	}()
	filename, ddl, err := g.readInputs(request)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, g.settings.GenerationTimeout)
	defer cancel()
	scratch, err := startScratch(ctx, g.settings, g.tracer)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), g.settings.CleanupTimeout)
		defer cleanupCancel()
		retErr = stderrors.Join(retErr, scratch.close(cleanupCtx))
	}()
	plan, err := scratch.buildPlan(ctx, request, ddl)
	if err != nil {
		return Result{}, err
	}
	if len(plan.Changes) == 0 {
		return Result{}, nil
	}
	files, err := sqltool.GooseFormatter.Format(plan)
	if err != nil {
		return Result{}, fmt.Errorf("format migration: %w", err)
	}
	if len(files) != 1 {
		return Result{}, fmt.Errorf("expected one Goose migration file")
	}
	file := files[0]
	if err := scratch.validateMigration(ctx, filename, file.Bytes()); err != nil {
		return Result{}, err
	}
	path := filepath.Join(request.MigrationsDir, filename)
	if err := writeMigration(path, file.Bytes()); err != nil {
		return Result{}, err
	}
	return Result{Path: path}, nil
}

func (g *Generator) readInputs(request Request) (filename string, ddl []byte, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.ErrValidationFailed.WithCause(retErr)
		}
	}()
	if err := g.settings.Validate(); err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(request.MigrationsDir)
	if err != nil {
		return "", nil, fmt.Errorf("read migration history: %w", err)
	}
	filenames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			filenames = append(filenames, entry.Name())
		}
	}
	filename, err = migrationFilename(request.MigrationName, g.now(), filenames)
	if err != nil {
		return "", nil, err
	}
	ddl, err = os.ReadFile(request.SchemaPath)
	if err != nil {
		return "", nil, fmt.Errorf("read desired schema: %w", err)
	}
	if len(ddl) == 0 {
		return "", nil, fmt.Errorf("desired schema must not be empty")
	}
	return filename, ddl, nil
}
