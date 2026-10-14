# Ingest

## Purpose
Turns confirmed uploads into stored document outlines, normalized blocks and table grids. M1.3a stops at stored blocks; ready does not yet mean vector indexed.

## Entry points
- `IngestPipeline.run` is invoked by the worker handler.
- `IngestJob.from_message` validates the stream payload.

## Dependencies
Uses Postgres, object storage, parsing and injected tracing/metrics. Used by the ingest worker.

## Run & test
```bash
uv run pytest -q tests/unit/test_outline.py tests/integration/test_ingest.py
```

## Conventions
Pending or failed files are claimed as parsing with a bounded lease; duplicate, finished, deleted or superseded jobs are skipped. Expired leases permit recovery. Failed attempts release the claim for stream retries; the dead letter hook records final failures. SHA256 is calculated while downloading the exact copy parsed. Temporary copies are removed after parsing. UUIDv5 heading nodes and transactional replacement make replay idempotent. Completion records ready or degraded with provider, quality and pages and clears the lease.

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
`summaries.SectionText` carries a node id and section text. `NodeSummarizer` resolves chat.fast only when enabled and given sections, takes the first configured maximum number of nodes, truncates each input by characters and passes the configured output token limit. Successful summaries retain section order and node ids. Missing roles and per-section provider failures leave summaries absent and never fail ingest; later sections continue after a failure. Cancellation still propagates for worker shutdown. Pipeline persistence and constructor wiring follow in Task 3b-6.
