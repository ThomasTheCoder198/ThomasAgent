-- +goose Up
-- Contract step (spec 2026-10-06 §3.5): M0.5.1 code uses request_id only.
ALTER TABLE outbox DROP COLUMN trace_parent;
ALTER TABLE audit_events DROP COLUMN trace_id;

-- +goose Down
ALTER TABLE audit_events ADD COLUMN trace_id text NOT NULL DEFAULT '';
ALTER TABLE outbox ADD COLUMN trace_parent text NOT NULL DEFAULT '';
