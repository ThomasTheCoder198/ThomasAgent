-- +goose Up
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     text        NOT NULL DEFAULT 'default',
    email         text        NOT NULL,
    password_hash text        NOT NULL,
    display_name  text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_users_tenant_email UNIQUE (tenant_id, email)
);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    text        NOT NULL DEFAULT 'default',
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   bytea       NOT NULL,
    csrf_token   text        NOT NULL,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_sessions_token_hash UNIQUE (token_hash)
);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

CREATE TABLE secrets (
    id         uuid PRIMARY KEY,
    tenant_id  text        NOT NULL DEFAULT 'default',
    key_id     text        NOT NULL,
    nonce      bytea       NOT NULL,
    ciphertext bytea       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE providers (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         text        NOT NULL DEFAULT 'default',
    kind              text        NOT NULL CHECK (kind IN ('openrouter','openai','anthropic','google','azure','ollama','openai_compatible','cohere','voyage','jina','typesafe')),
    name              text        NOT NULL,
    base_url          text        NOT NULL DEFAULT '',
    api_key_secret_id uuid        REFERENCES secrets (id) ON DELETE SET NULL,
    enabled           boolean     NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_providers_tenant_name UNIQUE (tenant_id, name)
);

CREATE TABLE models (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             text          NOT NULL DEFAULT 'default',
    provider_id           uuid          NOT NULL REFERENCES providers (id) ON DELETE CASCADE,
    model_ref             text          NOT NULL,
    display_name          text          NOT NULL,
    capabilities          text[]        NOT NULL DEFAULT '{}',
    context_window        integer,
    embedding_dims        integer,
    input_price_per_mtok  numeric(14,6),
    output_price_per_mtok numeric(14,6),
    source                text          NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','sync')),
    created_at            timestamptz   NOT NULL DEFAULT now(),
    updated_at            timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT uq_models_provider_ref UNIQUE (provider_id, model_ref)
);

CREATE TABLE model_roles (
    tenant_id  text        NOT NULL DEFAULT 'default',
    role       text        NOT NULL CHECK (role IN ('chat.default','chat.fast','vision','embedding','rerank','decision')),
    model_id   uuid        NOT NULL REFERENCES models (id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, role)
);

-- +goose Down
DROP TABLE model_roles;
DROP TABLE models;
DROP TABLE providers;
DROP TABLE secrets;
DROP TABLE sessions;
DROP TABLE users;
