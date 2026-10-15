# Retrieval

## Responsibilities
Vietnamese segmentation (S6/B11), offline BM25 encoding (B12), and tenant scoped Qdrant persistence (B1?B4).

## Interfaces
`Segmenter.segment` applies the same tokenizer to passages and queries. `create_segmenter` accepts `underthesea` or `none`. `SparseEncoder` lazily constructs FastEmbed BM25 from an empty local directory, without stemming or downloads; Qdrant applies IDF. Call synchronous encoding through `asyncio.to_thread`.

`QdrantStore` ensures one dense cosine and BM25 collection per embedding model and dimension, with keyword tenant indexes on `org_id` and `kb_id` and a file index. Replacing a file deletes its old points before batched upserts to avoid orphans.

## Tenancy and payload
`tenant_filter(org_id, kb_id, ...)` is the single filter entry point; both values are required on reads and deletes. Payloads contain org, KB, file, version, chunk identity/index, node, section, pages, block IDs and locator offsets, document type/name, language, clean text, and creation timestamp. Point IDs are deterministic UUIDv5 values derived from chunk IDs.

## Configuration and verification
Typed config supplies tokenizer, BM25 k/b/average length, Qdrant timeout and upsert batch size. Unit tests block sockets during BM25 construction; integration tests exercise collection idempotence, replacement, tenant isolation and locator payloads against Qdrant 1.19.1.

Indexed points carry org_id, kb_id, file_id, version, chunk_id/index, node_id, section, page_start/end, block offsets and locators, doc_type, file_name, lang, text and created_at. Dense and BM25 vectors share a deterministic UUIDv5 point id.


## Hybrid search
`SearchService.search(org_id, kb, query, filters)` embeds with the KB's stored embedding model,
encodes the query with the same BM25 segmenter as ingest, and calls `QdrantStore.hybrid_query`.
Dense and sparse prefetches (40 each) and the final RRF query all carry `tenant_filter(org_id, kb.id, ...)`.
Optional file IDs and document types only narrow that filter. A missing collection returns no candidates.
The 30 fused candidates are checked in Postgres before reranking: only non-deleted files in the org
with `ready` or `degraded` status remain, excluding superseded files pending purge (C3).

Role `rerank` is required even for an empty KB: an unassigned role raises `LLM_ROLE_NOT_ASSIGNED`.
A decision-only model raises `LLM_MODEL_CAPABILITY_MISMATCH` until M6 (C1).
Reranked scores below `RAG_SEARCH_MIN_RERANK_SCORE` (0.1) are removed; at most
`RAG_SEARCH_RESULT_LIMIT` (8) results remain. An empty answer sets `insufficient_evidence=true` (C4).
Core owns the corrective query rewrite in M2. Parent/neighbour expansion waits for M6; callers use
`node_id` to request sections (C5).

Citation labels are `[n.k]`, numbering results and their blocks from 1. Each block carries its
`block_id`, exact chunk text sliced by stored offsets, and stored locator (C6).
Spans `rag.retrieval.search` and `rag.retrieval.rerank` use OpenInference `RETRIEVER` and `RERANKER`.
Candidate/result counts are safe attributes; `input.value` contains the query only with
`RAG_TRACE_CONTENT=true`. Document content is never attached by these spans (C12).

`LiteLLMReranker(retrying_factory, metrics=recorder)` passes credentials per call, maps provider errors,
and uses platform retries including `Retry-After`. The required recorder implements
`observe_rerank_call(kind, outcome, duration_seconds)`; `WorkerMetrics` provides it through an injected
meter. Completion logs include request ID, duration and bounded outcome without query, documents,
credentials or provider exception text. API tool assembly must pass its configured exported recorder.

`create_search_service(config, resources, registry, tracer, metrics)` in `factory.py` is the single production construction of `SearchService`, used by the API and `ragx eval run`. It passes the configured `WorkerMetrics` recorder to the embedder and the reranker, so each entry point exports provider RED metrics. `BM25_DIR_NAME` lives there and ingest imports it, so queries and ingest share one BM25 directory.

Run `uv run pytest -q tests/integration/test_search.py tests/unit/test_rerank.py` from `ragx/`.
Missing rerank roles require an operator role assignment; deleted-file points are harmless while purge waits.


Rerank responses must contain unique integer indices within the input document bounds and finite numeric
scores. Booleans, coerced strings/fractional indices, duplicates and malformed result rows are rejected
as `LLM_PROVIDER_REJECTED` without retry or provider content in errors/logs. Search spans record ERROR
status without exception text, traceback events or status descriptions, so dependency errors cannot
export query/document/credential content through those fields.

The four KB tools use closed camelCase DTOs and `KbToolService`; the repository scopes every lookup and recursive step by org and KB. Outline and section reads require live ready/degraded files. Lists include confirmed statuses and use the shared file cursor encoding. Tool spans have bounded deadlines and suppress exception details.

`read_section` fetches the section in pages of `section_page_rows` blocks (keyset on `position`, `tool_repository.section_blocks`) and stops as soon as the character cap is reached, so it reads at most one page past the cap however long the section is (Codex I2). Section output preserves whole blocks where possible. An oversized first block is clipped to `section_max_chars` and marked `truncated`, so the configured character bound holds even for a single large block.
