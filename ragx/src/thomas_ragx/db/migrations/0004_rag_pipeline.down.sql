DROP TABLE embedding_cache;
DROP TABLE kb_tables;
DROP TABLE doc_blocks;
DROP TABLE doc_nodes;
ALTER TABLE kb_files
    DROP COLUMN ingested_at,
    DROP COLUMN ingest_lease_until,
    DROP COLUMN chunk_count,
    DROP COLUMN page_count,
    DROP COLUMN parse_quality,
    DROP COLUMN parse_provider;
ALTER TABLE knowledge_bases DROP COLUMN collection_name;
