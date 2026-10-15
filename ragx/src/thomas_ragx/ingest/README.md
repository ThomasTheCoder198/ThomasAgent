# Ingest

## Purpose
Turns confirmed uploads into stored document outlines, normalized blocks, table grids, searchable Qdrant points and preview JSON. Ready means indexed.

## Entry points
- `IngestPipeline.run` is invoked by the worker handler.
- `IngestJob.from_message` validates the stream payload.

## Dependencies
Uses Postgres, object storage, parsing and injected tracing/metrics. Used by the ingest worker.

## Run & test
```bash
uv run pytest -q tests/unit/test_outline.py tests/integration/test_ingest.py tests/integration/test_ingest_fencing.py
```

## Conventions
Pending or failed files are claimed as parsing with a bounded lease; duplicate, finished, deleted or superseded jobs are skipped. Expired leases permit recovery. Failed attempts release the claim for stream retries; the dead letter hook records final failures. SHA256 is calculated while downloading the exact copy parsed. Temporary copies are removed after parsing. If fetch is cancelled while MinIO's synchronous download is running, the storage callback removes the attempt-specific copy after the writer thread closes. The pipeline knows that path before awaiting fetch, so it also removes copies when cancellation happens before parsing. UUIDv5 heading nodes and transactional replacement make replay idempotent. Completion records ready or degraded with provider, quality and pages and clears the lease.

`rag.ingest.fetch`, `rag.ingest.parse`, `rag.ingest.persist` and parser spans carry request_id, org_id, kb_id and file_id. Step durations and file outcomes use bounded metric labels; no tenant or request labels.

## Common failures
- Changed object bytes produce INGEST_CONTENT_CHANGED before parsing.
- Corrupt documents produce INGEST_PARSE_FAILED; absent text produces INGEST_NO_TEXT.

Step duration metrics include unsuccessful fetch, parse and persist attempts; only successful steps emit completion logs.

Chunker.chunk converts blocks and their outline into deterministic chunks. Word counts use whitespace tokens (Vietnamese syllables). A section node is a hard boundary; blocks join until the target is reached or the next block would exceed the maximum. Tables occupy their own chunks; oversized tables split by complete rows with repeated headers and narrowed sheet ranges. Oversized text uses maximum-size windows with configured overlap. Block offsets index the chunk body with exclusive ends. Embedding text adds document, optional section and page metadata.

Chunk ids are sha256("{file_id}:{version}:{index}")[:32]; point ids are UUIDv5 using the fixed CHUNK_NAMESPACE. Never `hash()`. Language is vi when Vietnamese letters occur, otherwise en.

Tables with only a header stay intact even when that header exceeds the chunk maximum, because no body-row boundary exists to split at.

## Embedding cache
`embedding_cache.CachedEmbedder` embeds missing content in configured batches and stores little-endian float32 vectors keyed by `(model_id, sha256(embed_text))`. The cache is shared across orgs; callers pass the KB-selected handle and dimensions. Input order is preserved, duplicate misses are embedded once per call, and per-input hit/miss counts are returned and recorded. Wrong vector lengths raise non-retryable `INGEST_EMBEDDING_MISMATCH`. Run `uv run pytest -q tests/integration/test_embedding_cache.py`.

Warm embedding cache reads validate both persisted dimension metadata and decoded vector lengths against the KB dimensions before returning any vectors. Hit counting uses a set of missing keys to stay linear in input size.

## Optional section summaries
`summaries.SectionText` carries a node id and section text. `NodeSummarizer` resolves chat.fast only when enabled and given sections, takes the first configured maximum number of nodes, truncates each input by characters and passes the configured output token limit. Successful summaries retain section order and node ids. Missing roles and per-section provider failures leave summaries absent and never fail ingest; later sections continue after a failure. Cancellation still propagates for worker shutdown. The pipeline persists successful summaries during enrichment; worker construction injects the configured chat adapter.

## Indexed readiness

The pipeline claims and fetches the verified source, parses, persists the outline, enriches section summaries, chunks, embeds, indexes dense and BM25 points, writes preview JSON, then finishes. Temporary raw copies are removed after parsing. Each step has a CHAIN span and duration metric; embedding/cache/index spans contain counts only.

The embedding model and dimensions come from the KB snapshot, never the current embedding role (B5). Optional chat.fast summaries cannot fail ingest (B14). Finishing retires older versions of the same name and enqueues their purge in the same transaction (B15). Publication validates KB/file liveness under the KB lock; finalization removes points and preview when file deletion wins (B16). Preview JSON contains fileId, version, provider, pageCount and blocks with blockId, nodeId, type, text and locator (B17).

## Attempts, fencing and the job deadline (Codex I1)
Each claim writes a new `kb_files.ingest_attempt_id` and returns it on `IngestTarget.attempt_id`. Every write of an attempt matches that id: `set_step`, `release_claim`, `finish`, `set_summaries` and the outline write. A stale attempt (its lease expired and another attempt claimed the file) therefore changes nothing in Postgres.

`_persist` and `_publish` call `claim_state` under the KB lock before any outline, Qdrant or preview write: `owned` proceeds; `gone` (file or KB deleted) keeps the B16 cleanup; `lost` (live, but another attempt holds or finished it) stops with no writes and no cleanup, and is logged as `ingest abandoned` with `claim=lost`. Cleanup after a lost finish or a failed index/preview stays safe because ownership was confirmed under the KB lock that every other persist, publish and purge also takes: no one else can have published this file's points (whose ids are deterministic per file, version and chunk) or preview meanwhile.

An attempt runs under `asyncio.timeout(Config.ingest_job_deadline_seconds)` = ingest visibility timeout − stream visibility timeout (1740 s by default), so it ends before its lease can expire. When the deadline fires the pipeline logs one warning `ingest deadline exceeded` (`request_id`, `file_id`, `deadline_seconds`, `duration_ms`; no content), so "this file is too slow" can be told apart from a provider timeout, then raises retryable `DEPENDENCY_TIMEOUT`, releases the claim and follows the normal retry/DLQ path; publication cleanup also runs on cancellation, so a partial index is removed. A file that needs longer than the deadline fails; raise `RAG_INGEST_VISIBILITY_TIMEOUT_MILLISECONDS` to give it more (lease and deadline move together). The raw temp copy's name includes the attempt id. The DB lease is `Config.ingest_lease_seconds` = visibility − half the stream visibility margin (1770 s): it ends after the deadline (cleanup window) and before Redis may reclaim the entry, so a crash-reclaimed entry finds an expired lease instead of skipping and being acknowledged (m3). An in-progress file whose attempt vanished anyway is re-driven by the worker sweep (`worker.recovery.IngestRecovery`).

Publication holds the KB row lock across indexing, preview and finish, using the same KB-before-file lock order as purge. Outline replacement also locks the KB first so FK insertion cannot invert this order. Liveness includes the KB tombstone. An indexing/preview failure or lost finalization removes points and preview while the lock is held. Provider calls and chunking remain outside this transaction. Holding a DB connection/KB lock across external storage calls serializes publication per KB and can delay KB deletion.
