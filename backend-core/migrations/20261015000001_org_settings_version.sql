-- +goose Up
ALTER TABLE organizations ADD COLUMN settings_version bigint NOT NULL DEFAULT 1;
ALTER TABLE organizations ADD CONSTRAINT ck_organizations_settings_version_positive CHECK (settings_version > 0);

-- +goose Down
ALTER TABLE organizations DROP CONSTRAINT ck_organizations_settings_version_positive;
ALTER TABLE organizations DROP COLUMN settings_version;
