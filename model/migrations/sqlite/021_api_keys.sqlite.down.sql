-- Restore the oldest key into the legacy settings row (the one carried over by
-- the up migration, if any), then drop the table.
INSERT INTO settings (name, value)
SELECT 'api_key', key_hash FROM api_keys ORDER BY id ASC LIMIT 1;

DROP TABLE api_keys;
