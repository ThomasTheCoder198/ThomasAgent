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
