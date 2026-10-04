# Audit

## Purpose

Persists metadata-only audit events and copies the trace id from context.
M0.2 data belongs to the [Platform tenant](../../../docs/glossary.md) (`default`), using the migration's platform defaults. M1 business tenancy must take `tenant_id` from trusted context, never client or model parameters.

## Entry points

- `Record(ctx, db, Event)` is called by domain services; `Event` carries actor, action, target and metadata.

## Dependencies

- Uses: `postgres.DBTX`, JSON encoding, and OpenTelemetry trace context.
- Used by: core domain services and the forthcoming HTTP/startup wiring.

## Run & test

```bash
cd backend-core
go test ./internal/audit/... -count=1 -p 2
```

Tests use `pgtest.Start`, migrated Docker Postgres and Ryuk.

## Conventions

Passwords, raw session tokens and secrets must never enter logs, errors or audit metadata. Errors propagate to the boundary for one log and shared HTTP formatting.

## Common failures

- Unsupported JSON metadata or a database insert failure returns an error to the caller.
