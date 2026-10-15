# Ragx integration tests

## Purpose
Integration tests against real Postgres, Redis and MinIO through testcontainers.

## Entry points
- `test_migrations.py`: tests migration pairing and up/down behavior.
- `conftest.py`: shared containers and an isolated pool fixture.
- `api_client.py`: scoped App token and ASGI client for API tests.
- `test_files_api.py`: presign, POST to MinIO, confirm, dedupe, download-url and tenant isolation. The helpers in `api_client.py` (`presign`, `post_to_storage`, `confirm`, `upload`) do a real multipart POST to the MinIO container, as a browser does; `api_client` signs for the container's address.
- `test_eval_harness.py`: the eval runner on the indexed synthetic corpus with fake models; recall, MRR, locator accuracy and ingest throughput.
- `test_tool_isolation.py`: end to end over the four KB tools. Two orgs index the same corpus; org A's token never returns, outlines or reads org B's chunks, nodes or files (B's KB claim, B's ids in the body, a `fileIds` filter naming B's file, with positive controls for A's own file and listing), a body cannot name a KB or org, and an unprovisioned org is refused. A second case puts two KBs with the same corpus in org A (one shared Qdrant collection): a token for one KB never searches, lists, outlines or reads the other KB's files and nodes, and the tenant filter of one KB counts none of the other's points (with positive controls).
- `test_ingest_fencing.py`: two attempts of one file (Codex I1). A's lease expires while it is paused in parse or embed, B finishes the file, then A resumes: the file stays `ready` with its points, preview and outline, and A never reaches its index. An owner whose finish loses to a takeover mid-index removes only its own points and preview, and B then completes. A failing stale attempt neither releases B's in-progress claim nor lets the dead-letter hook fail it. The job deadline cancels a hung index, removes the partial points and releases the claim with `DEPENDENCY_TIMEOUT`, logging one `ingest deadline exceeded` warning.
- `test_ingest_recovery.py`: stalled ingest recovery (m1/m3). A crashed attempt whose redelivery met its live lease and was acknowledged is untouched within the grace, requeued after it and ends `ready` through the outbox and consumer; a file past the recovery limit ends `failed` with a DLQ entry, and an explicit retry resets its budget; live leases and deleted files are left alone.
- `test_section_paging.py`: `kb_read_section` over a 300-block section with a nested sub-section reads pages of `section_page_rows` and stops one page past the character cap, with output identical to reading the whole section (Codex I2).
- `test_dlq_replay.py`: a provider outage is delivered `MAX_DELIVERIES` times (pinned by the DLQ entry's `deliveries` field and the embedder's failed calls) before the DLQ entry and the `failed` mark; `DeadLetterReplayer` puts it back and the file ends `ready` with its stale error code cleared. A corrupt file replays into the same `INGEST_PARSE_FAILED`, with one entry left in the DLQ.
- `ingest_support.py` and `search_support.py`: shared helpers for these files (`uploaded_file`, `file_row`, `pipeline_for`, `indexed_corpus`, `call_tool`, `tool_service_for`).
- `test_kb_api.py`: provisioning, tenant isolation, Knowledge Base CRUD and manifest tests.

## Dependencies
- Uses: pytest, testcontainers and Docker.
- Used by: ragx test suite and CI.

## Run & test
```bash
cd ragx
uv run pytest -q tests/integration
```

On Windows, set `DOCKER_HOST=npipe:////./pipe/docker_engine` and `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=//var/run/docker.sock` first.

## Conventions
Images match deploy/ragx; the pool fixture truncates every table before each test.

## Common failures
- Docker not running → start Docker Desktop.
- Rootless Docker message on Windows → set `DOCKER_HOST` as documented.

The session Qdrant fixture uses `qdrant/qdrant:v1.19.1-unprivileged` and a 30-second client timeout; each test clears collections. `api_client` accepts `qdrant_url`, otherwise its Qdrant endpoint is unreachable.
