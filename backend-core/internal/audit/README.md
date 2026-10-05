# Audit

## Purpose

Persists metadata-only audit events and copies the trace id from context. Audit writes require the trusted tenant from context. Missing scope returns the catalog `INTERNAL_ERROR`; an event tenant never overrides context. Nil metadata is stored as `{}`.
Explicit platform boundaries set the [Platform tenant](../../../docs/glossary.md) (`default`); business data always uses trusted context, never client or model parameters.

## Entry points

- `Record(ctx, db, Event)` is called by domain services; `Event` carries actor, action, target and metadata.

## Dependencies

- Uses: `postgres.DBTX`, JSON encoding, and OpenTelemetry trace context.
- Used by: core domain services and the forthcoming HTTP/startup wiring.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/audit/... -count=1 -p 2
```

Tests use `pgtest.Start` and migrated Docker Postgres with Ryuk disabled on this host.

## Conventions

Passwords, raw session tokens and secrets must never enter logs, errors or audit metadata. Errors propagate to the boundary for one log and shared HTTP formatting.

## Common failures

- Unsupported JSON metadata or a database insert failure returns an error to the caller.
