ALTER TABLE orgs DROP CONSTRAINT ck_orgs_settings_version_nonnegative;
ALTER TABLE orgs DROP COLUMN settings_version;
