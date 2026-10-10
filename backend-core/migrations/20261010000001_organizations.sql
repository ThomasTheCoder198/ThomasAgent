-- +goose Up
-- An organization is platform data (tenant_id = 'default'); its own id is the tenant id of its business data and
-- the org_id in ragx (spec ragx-custom-app §2.1).
CREATE TABLE organizations (
    id                 text        PRIMARY KEY,
    tenant_id          text        NOT NULL,
    name               text        NOT NULL,
    external_parsing   boolean     NOT NULL,
    ragx_status        text        NOT NULL DEFAULT 'pending',
    ragx_error_code    text        NOT NULL DEFAULT '',
    ragx_default_kb_id uuid,
    settings_synced    boolean     NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_organizations_ragx_status CHECK (ragx_status IN ('pending', 'ready', 'failed')),
    -- ragx's org id pattern; 'default' is the platform tenant and can never be an organization.
    CONSTRAINT ck_organizations_id CHECK (id ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' AND id <> 'default')
);
CREATE UNIQUE INDEX uq_organizations_tenant_id_name ON organizations (tenant_id, lower(name));

-- +goose Down
DROP TABLE organizations;
