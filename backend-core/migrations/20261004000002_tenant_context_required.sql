-- +goose Up
-- Trusted application boundaries now pass tenant_id on every business INSERT.
ALTER TABLE outbox ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE audit_events ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE users ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE sessions ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE secrets ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE providers ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE models ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE model_roles ALTER COLUMN tenant_id DROP DEFAULT;

-- +goose Down
-- Restores the legacy schema contract without changing existing row tenants.
ALTER TABLE outbox ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE audit_events ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE users ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE sessions ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE secrets ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE providers ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE models ALTER COLUMN tenant_id SET DEFAULT 'default';
ALTER TABLE model_roles ALTER COLUMN tenant_id SET DEFAULT 'default';
