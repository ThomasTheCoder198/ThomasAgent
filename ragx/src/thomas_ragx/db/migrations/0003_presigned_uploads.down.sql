-- Unconfirmed uploads cannot exist in the old schema; their objects (if any) stay until the KB is purged.
DELETE FROM kb_files WHERE status = 'uploading';
ALTER TABLE kb_files DROP CONSTRAINT ck_kb_files_uploading_shape;
ALTER TABLE kb_files ALTER COLUMN version SET NOT NULL;
ALTER TABLE kb_files ALTER COLUMN sha256 SET NOT NULL;
ALTER TABLE kb_files DROP CONSTRAINT ck_kb_files_status;
ALTER TABLE kb_files ADD CONSTRAINT ck_kb_files_status CHECK (
    status IN ('pending', 'parsing', 'enriching', 'chunking', 'embedding', 'indexing', 'ready', 'failed', 'degraded')
);
