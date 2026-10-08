# Ragx database

## Purpose
Owns ragx's Postgres: the async pool, the migration runner, SQL migrations and transactional outbox writer.

## Entry points
- `create_pool`: creates a closed async pool for the app lifecycle.
- `migrate_up`, `migrate_down`: apply or revert paired SQL migrations.
- `ragx migrate up|down --steps N`: operator commands.
- `ragx bootstrap`: runs `migrate_up` before checking storage.
- `enqueue_outbox`: writes a job in the caller's business transaction.

## Dependencies
- Uses: psycopg 3.3.6 and psycopg-pool 3.3.3.
- Used by: resources, services and bootstrap.

## Run & test
```bash
cd ragx
uv run pytest -q tests/integration/test_migrations.py
```

## Conventions
Migrations are `NNNN_name.up.sql` and `.down.sql` pairs. Never edit an applied migration; breaking changes use expand/contract (main spec §18.8). The runner holds one advisory lock and applies each migration in its own transaction. SQL files are bytes so psycopg runs multi-statement files without parameters.

Call `enqueue_outbox` before committing the associated business change. The worker relay publishes committed rows to Redis Streams.

## Common failures
- Advisory lock wait → another bootstrap is migrating.
- Pair missing → add the corresponding `.down.sql`.
