-- +goose Up
ALTER TABLE outbox ADD COLUMN traceparent text;

-- +goose Down
ALTER TABLE outbox DROP COLUMN traceparent;
