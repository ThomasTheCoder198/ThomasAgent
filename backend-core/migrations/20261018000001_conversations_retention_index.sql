-- +goose Up
CREATE INDEX idx_conversations_updated_at ON conversations (updated_at);

-- +goose Down
DROP INDEX idx_conversations_updated_at;
