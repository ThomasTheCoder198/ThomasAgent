-- +goose Up
-- The worker, not the API, picks the model and words the status labels (amendment W), so the request's choices travel
-- on the run row: requested_model_id is the composer's model (B2-8; NULL = the Agent's override or the role) and
-- locale is the request's UI language (B2-9). requested_model_id has no foreign key: models are platform rows and a
-- deleted one ends the run with a catalog error instead of blocking the delete.
ALTER TABLE runs
    ADD COLUMN requested_model_id uuid,
    ADD COLUMN locale text NOT NULL DEFAULT 'vi' CHECK (locale IN ('vi', 'en'));

-- +goose Down
ALTER TABLE runs
    DROP COLUMN locale,
    DROP COLUMN requested_model_id;
