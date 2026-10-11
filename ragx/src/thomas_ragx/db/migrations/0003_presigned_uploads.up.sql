-- Presigned uploads (spec ragx-custom-app §14.3): a file row exists from presign on, as 'uploading', before its
-- bytes are known. sha256 and version are set at confirm (ruling P9), so concurrent presigns of one name never
-- collide on uq_kb_files_kb_id_name_version or uq_kb_files_kb_id_sha256 (NULLs are distinct).
ALTER TABLE kb_files DROP CONSTRAINT ck_kb_files_status;
ALTER TABLE kb_files ADD CONSTRAINT ck_kb_files_status CHECK (
    status IN ('uploading', 'pending', 'parsing', 'enriching', 'chunking', 'embedding', 'indexing', 'ready', 'failed', 'degraded')
);
ALTER TABLE kb_files ALTER COLUMN sha256 DROP NOT NULL;
ALTER TABLE kb_files ALTER COLUMN version DROP NOT NULL;
ALTER TABLE kb_files ADD CONSTRAINT ck_kb_files_uploading_shape CHECK (
    (status = 'uploading') = (sha256 IS NULL) AND (status = 'uploading') = (version IS NULL)
);
