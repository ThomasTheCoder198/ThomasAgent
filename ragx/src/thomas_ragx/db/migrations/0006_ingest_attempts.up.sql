-- Fencing token of the ingest attempt that holds the file (Codex I1): every write of an attempt matches it, so an
-- attempt whose lease expired and was taken over changes nothing. ingest_recoveries counts how often the worker's
-- sweep re-drove a file whose attempt vanished (lease long expired, no stream entry left); past the delivery limit
-- the sweep fails the file instead.
ALTER TABLE kb_files
    ADD COLUMN ingest_attempt_id uuid,
    ADD COLUMN ingest_recoveries integer NOT NULL DEFAULT 0;

-- kb_read_section reads a section in position order a page at a time and stops at its character cap (Codex I2).
CREATE INDEX idx_doc_blocks_file_id_position ON doc_blocks (file_id, position);
