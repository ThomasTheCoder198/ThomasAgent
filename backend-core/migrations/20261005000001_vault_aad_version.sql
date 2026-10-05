-- +goose Up
ALTER TABLE secrets DROP CONSTRAINT secrets_aad_version_check;
ALTER TABLE secrets ADD CONSTRAINT secrets_aad_version_check CHECK (aad_version IN (1, 2, 3));

-- +goose Down
-- SQL has no encryption key to reseal version 3 rows. Keep the expanded check
-- and marker on rollback so existing ciphertext remains correctly identified.
SELECT 1;
