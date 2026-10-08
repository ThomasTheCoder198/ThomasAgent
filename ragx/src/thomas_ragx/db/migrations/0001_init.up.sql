CREATE TABLE orgs (
    id               text        PRIMARY KEY,
    name             text        NOT NULL,
    external_parsing boolean     NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE providers (
    id                 uuid        PRIMARY KEY,
    kind               text        NOT NULL,
    name               text        NOT NULL,
    base_url           text        NOT NULL DEFAULT '',
    api_key_nonce      bytea,
    api_key_ciphertext bytea,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_providers_name UNIQUE (name),
    CONSTRAINT ck_providers_api_key_pair CHECK ((api_key_nonce IS NULL) = (api_key_ciphertext IS NULL))
);

CREATE TABLE models (
    id                    uuid        PRIMARY KEY,
    provider_id           uuid        NOT NULL REFERENCES providers (id) ON DELETE RESTRICT,
    name                  text        NOT NULL,
    capabilities          text[]      NOT NULL DEFAULT '{}',
    capabilities_edited   boolean     NOT NULL DEFAULT false,
    embedding_dims        integer,
    context_window        integer,
    input_cost_per_token  numeric,
    output_cost_per_token numeric,
    source                text        NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_models_provider_id_name UNIQUE (provider_id, name),
    CONSTRAINT ck_models_source CHECK (source IN ('sync', 'manual'))
);

CREATE TABLE model_roles (
    role       text        PRIMARY KEY,
    model_id   uuid        NOT NULL REFERENCES models (id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE knowledge_bases (
    id                 uuid        PRIMARY KEY,
    org_id             text        NOT NULL REFERENCES orgs (id) ON DELETE RESTRICT,
    name               text        NOT NULL,
    is_default         boolean     NOT NULL DEFAULT false,
    embedding_model_id uuid        NOT NULL REFERENCES models (id) ON DELETE RESTRICT,
    embedding_dims     integer     NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE UNIQUE INDEX uq_knowledge_bases_org_id_name ON knowledge_bases (org_id, name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_knowledge_bases_org_id_default ON knowledge_bases (org_id) WHERE is_default;

CREATE TABLE kb_files (
    id           uuid        PRIMARY KEY,
    org_id       text        NOT NULL,
    kb_id        uuid        NOT NULL REFERENCES knowledge_bases (id) ON DELETE CASCADE,
    name         text        NOT NULL,
    version      integer     NOT NULL,
    content_type text        NOT NULL,
    size_bytes   bigint      NOT NULL,
    sha256       text        NOT NULL,
    status       text        NOT NULL DEFAULT 'pending',
    error_code   text        NOT NULL DEFAULT '',
    degraded     boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    CONSTRAINT ck_kb_files_status CHECK (
        status IN ('pending', 'parsing', 'enriching', 'chunking', 'embedding', 'indexing', 'ready', 'failed', 'degraded')
    )
);
CREATE UNIQUE INDEX uq_kb_files_kb_id_sha256 ON kb_files (kb_id, sha256) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_kb_files_kb_id_name_version ON kb_files (kb_id, name, version);
CREATE INDEX idx_kb_files_kb_id_created_at_id ON kb_files (kb_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE outbox (
    id           uuid        PRIMARY KEY,
    org_id       text        NOT NULL,
    stream       text        NOT NULL,
    payload      jsonb       NOT NULL,
    request_id   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX idx_outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;

CREATE TABLE idempotency_keys (
    org_id     text        NOT NULL,
    operation  text        NOT NULL,
    key        text        NOT NULL,
    file_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, operation, key)
);
