-- +goose Up
-- The lease makes a chat run execute at most once (amendment W5): lease_token is unique per claim attempt, so a stale
-- executor's writes are fenced out. request_id is the X-Request-Id of the chat request that started the run (W9).
ALTER TABLE runs
    ADD COLUMN lease_owner text,
    ADD COLUMN lease_token uuid,
    ADD COLUMN lease_until timestamptz,
    ADD COLUMN request_id text NOT NULL DEFAULT '';
-- ConversationService.Get and the stop/relay reads pick the latest run of a conversation.
CREATE INDEX runs_conversation_started ON runs (conversation_id, started_at DESC);

-- +goose Down
DROP INDEX runs_conversation_started;
ALTER TABLE runs
    DROP COLUMN request_id,
    DROP COLUMN lease_until,
    DROP COLUMN lease_token,
    DROP COLUMN lease_owner;
