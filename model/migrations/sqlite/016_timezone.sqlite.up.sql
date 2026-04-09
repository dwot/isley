-- Seed default timezone settings into the key-value settings table.
-- "timezone" is the user-facing setting (IANA identifier, e.g. "America/New_York").
-- The tz_* keys are shadow metadata captured automatically to inform future migration.
INSERT OR IGNORE INTO settings (name, value) VALUES ('timezone', '');
INSERT OR IGNORE INTO settings (name, value) VALUES ('tz_system', '');
INSERT OR IGNORE INTO settings (name, value) VALUES ('tz_database', '');
INSERT OR IGNORE INTO settings (name, value) VALUES ('tz_user', '');
INSERT OR IGNORE INTO settings (name, value) VALUES ('tz_snapshot_at', '');
