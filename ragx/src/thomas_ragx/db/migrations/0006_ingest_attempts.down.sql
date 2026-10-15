DROP INDEX idx_doc_blocks_file_id_position;
ALTER TABLE kb_files
    DROP COLUMN ingest_recoveries,
    DROP COLUMN ingest_attempt_id;
