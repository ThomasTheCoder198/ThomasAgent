-- +goose Up
ALTER TABLE users ADD CONSTRAINT uq_users_tenant_id UNIQUE (tenant_id, id);
ALTER TABLE sessions DROP CONSTRAINT sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT fk_sessions_tenant_user
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE;

-- Existing ciphertext used the row UUID as AAD. New writes explicitly use version 2.
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS aad_version smallint NOT NULL DEFAULT 1 CHECK (aad_version IN (1, 2));

-- +goose Down
-- Preserve the format marker: rolling back a constraint must not mislabel upgraded ciphertext.
ALTER TABLE sessions DROP CONSTRAINT fk_sessions_tenant_user;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE users DROP CONSTRAINT uq_users_tenant_id;
