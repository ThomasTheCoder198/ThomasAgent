# Migration generator command

## Purpose
Exposes schema-to-Goose generation for development. Prints a created file path or
a no-change message, and reports failures once through structured logging.

## Entry points
- `main` handles process signals and exit status.
- `runMigrationGenerator` loads only schema tooling settings and invokes the injected generator.
- Flags: `-name` (required snake_case), `-schema` (schema.sql), `-migrations` (migrations).

## Dependencies
- Uses: config, logging, schemadiff, canonical `internal/errors` definitions and OTEL spans.
- Used by: the `migration:diff` task and local developers.

## Run & test
```bash
task migration:diff NAME=add_feature
cd backend-core
go run ./cmd/migrationgen -name add_feature
go test ./cmd/migrationgen -count=1
```
Docker Desktop must run. Schema settings load from an optional .env using the
same precedence as core. No CORE_DATABASE_URL or CORE_REDIS_URL is required.

## Conventions
Success exits 0; failures exit 1 and produce one JSON error with a trace_id.
Generation leaves the development database and deployment stack unchanged.
The command uses an in-process tracer without an exporter; schema generation
does not require an OTLP collector. Shutdown and cleanup have bounded deadlines.

## Common failures
- Missing name or invalid flags: provide `-name add_feature`.
- Invalid schema or unsupported objects: see the schemadiff README.
- Docker unavailable: start Docker Desktop before generating migrations.
