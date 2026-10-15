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

Migration 0004 (`rag_pipeline`, M1.3) adds:
- `knowledge_bases.collection_name`: the KB's Qdrant collection, `kb_<embedding model id hex>_<dims>`, backfilled for existing rows;
- `kb_files` columns `parse_provider`, `parse_quality`, `page_count`, `chunk_count`, `ingest_lease_until`, `ingested_at`;
- `doc_nodes`: the document tree (one row per section, `parent_id`, `position`, `depth`, `title`, `summary`);
- `doc_blocks`: one row per normalized block with its `locator` JSON, keyed by `(file_id, block_id)` and ordered inside a node by `position`;
- `kb_tables`: table JSON (`header`, `rows`) with its locator;
- `embedding_cache`: vectors keyed by `(model_id, content_sha256)`, shared across orgs on purpose.

The new tables reference `kb_files` with `ON DELETE CASCADE`, so purging a file removes its tree, blocks and tables.

Call `enqueue_outbox` before committing the associated business change. The worker relay publishes committed rows to Redis Streams.

## Common failures
- Advisory lock wait → another bootstrap is migrating.
- Pair missing → add the corresponding `.down.sql`.

Migration 0005 indexes doc_nodes.parent_id and kb_tables.node_id for foreign key cascade traversal during outline replacement and purge. The parent index also supports section traversal. This trades insert/storage overhead for bounded child lookup; its down migration removes both indexes.

Migration 0006 (`ingest_attempts`) adds `kb_files.ingest_attempt_id`, the fencing token of the ingest attempt holding the file (Codex I1), `kb_files.ingest_recoveries` (how often the worker sweep re-drove a stalled ingest; reset by an explicit retry), and the index `idx_doc_blocks_file_id_position` that `kb_read_section` pages on (Codex I2). Its down migration drops all three.
