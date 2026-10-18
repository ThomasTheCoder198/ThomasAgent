-- +goose Up
CREATE TABLE agents (
    id              uuid        PRIMARY KEY,
    tenant_id       text        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    description     text        NOT NULL DEFAULT '',
    instructions    text        NOT NULL DEFAULT '',
    kb_id           uuid        NOT NULL,
    model_overrides jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_agents_name CHECK (char_length(name) BETWEEN 1 AND 80),
    CONSTRAINT ck_agents_description CHECK (char_length(description) <= 280),
    CONSTRAINT ck_agents_instructions CHECK (char_length(instructions) <= 8000),
    CONSTRAINT uq_agents_tenant_name UNIQUE (tenant_id, name),
    -- Target of the composite foreign keys below: a child row can only name a parent of its own tenant.
    CONSTRAINT uq_agents_id_tenant UNIQUE (id, tenant_id)
);
CREATE INDEX idx_agents_tenant_created ON agents (tenant_id, created_at);

-- An operator's capability edit (Settings "Sửa", O8) survives every later sync, like ragx's capabilities_edited.
ALTER TABLE models ADD COLUMN capabilities_edited boolean NOT NULL DEFAULT false;

CREATE TABLE conversations (
    id                  uuid        PRIMARY KEY,
    tenant_id           text        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    agent_id            uuid        NOT NULL,
    user_id             uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title               text        NOT NULL DEFAULT '',
    summary             text        NOT NULL DEFAULT '',
    summary_through_seq bigint      NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_conversations_id_tenant UNIQUE (id, tenant_id),
    CONSTRAINT fk_conversations_agent FOREIGN KEY (agent_id, tenant_id) REFERENCES agents (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX idx_conversations_tenant_updated ON conversations (tenant_id, updated_at DESC, id DESC);

CREATE TABLE messages (
    conversation_id uuid        NOT NULL,
    id              text        NOT NULL,
    tenant_id       text        NOT NULL,
    seq             bigint      NOT NULL,
    role            text        NOT NULL,
    parts           jsonb       NOT NULL,
    metadata        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_messages PRIMARY KEY (conversation_id, id),
    CONSTRAINT uq_messages_conversation_seq UNIQUE (conversation_id, seq),
    CONSTRAINT fk_messages_conversation FOREIGN KEY (conversation_id, tenant_id) REFERENCES conversations (id, tenant_id) ON DELETE CASCADE,
    CONSTRAINT ck_messages_id CHECK (char_length(id) BETWEEN 1 AND 128),
    CONSTRAINT ck_messages_role CHECK (role IN ('user', 'assistant'))
);

CREATE TABLE runs (
    id                   uuid        PRIMARY KEY,
    tenant_id            text        NOT NULL,
    conversation_id      uuid        NOT NULL,
    assistant_message_id text        NOT NULL,
    status               text        NOT NULL,
    route                text        NOT NULL DEFAULT '',
    model_ref            text        NOT NULL DEFAULT '',
    error_code           text        NOT NULL DEFAULT '',
    input_tokens         bigint      NOT NULL DEFAULT 0,
    output_tokens        bigint      NOT NULL DEFAULT 0,
    deadline_at          timestamptz NOT NULL,
    started_at           timestamptz NOT NULL DEFAULT now(),
    finished_at          timestamptz,
    CONSTRAINT uq_runs_id_tenant UNIQUE (id, tenant_id),
    CONSTRAINT fk_runs_conversation FOREIGN KEY (conversation_id, tenant_id) REFERENCES conversations (id, tenant_id) ON DELETE CASCADE,
    CONSTRAINT ck_runs_status CHECK (status IN ('running', 'completed', 'failed', 'cancelled'))
);
CREATE UNIQUE INDEX uq_runs_one_active ON runs (conversation_id) WHERE status = 'running';

CREATE TABLE run_steps (
    run_id        uuid        NOT NULL,
    step          int         NOT NULL,
    tenant_id     text        NOT NULL,
    kind          text        NOT NULL,
    name          text        NOT NULL DEFAULT '',
    outcome       text        NOT NULL,
    error_code    text        NOT NULL DEFAULT '',
    input_tokens  bigint      NOT NULL DEFAULT 0,
    output_tokens bigint      NOT NULL DEFAULT 0,
    duration_ms   bigint      NOT NULL,
    started_at    timestamptz NOT NULL,
    CONSTRAINT pk_run_steps PRIMARY KEY (run_id, step),
    CONSTRAINT fk_run_steps_run FOREIGN KEY (run_id, tenant_id) REFERENCES runs (id, tenant_id) ON DELETE CASCADE,
    CONSTRAINT ck_run_steps_kind CHECK (kind IN ('router', 'model', 'tool', 'rewrite', 'citation_check', 'compaction', 'title')),
    CONSTRAINT ck_run_steps_outcome CHECK (outcome IN ('success', 'error', 'skipped'))
);

-- +goose Down
DROP TABLE run_steps;
DROP TABLE runs;
DROP TABLE messages;
DROP TABLE conversations;
DROP TABLE agents;
ALTER TABLE models DROP COLUMN capabilities_edited;
