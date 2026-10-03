-- +goose Up
CREATE TABLE outbox (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     text        NOT NULL DEFAULT 'default',
    topic         text        NOT NULL,
    payload       jsonb       NOT NULL,
    trace_parent  text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    published_at  timestamptz
);
CREATE INDEX idx_outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;

CREATE TABLE audit_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text        NOT NULL DEFAULT 'default',
    actor       text        NOT NULL,
    action      text        NOT NULL,
    target_type text        NOT NULL,
    target_id   text        NOT NULL DEFAULT '',
    metadata    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    trace_id    text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_events_created_at ON audit_events (created_at DESC);

-- +goose Down
DROP TABLE audit_events;
DROP TABLE outbox;
