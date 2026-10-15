-- Existing orgs start below core's first version so the next PUT refreshes any stale settings.
ALTER TABLE orgs ADD COLUMN settings_version bigint NOT NULL DEFAULT 0;
ALTER TABLE orgs ADD CONSTRAINT ck_orgs_settings_version_nonnegative CHECK (settings_version >= 0);
