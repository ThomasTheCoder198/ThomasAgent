-- +goose Up
-- Expand step (spec 2026-10-06 §3.5): code reads and writes only these columns; M0.5.2 drops the old trace ones.
ALTER TABLE outbox ADD COLUMN request_id text NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN request_id text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE audit_events DROP COLUMN request_id;
ALTER TABLE outbox DROP COLUMN request_id;
