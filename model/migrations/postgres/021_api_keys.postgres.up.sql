-- API access keys. Replaces the single hashed api_key row in settings with a
-- dedicated table so several keys can coexist (one per device or integration)
-- and be revoked independently. Only the bcrypt hash is stored; the plaintext
-- is shown once at creation and is unrecoverable afterwards.
--
-- prefix holds the first few characters of the plaintext so a key can be told
-- apart in the UI and so auth can narrow the bcrypt comparison to one candidate
-- row instead of hashing against every key. The leftover entropy (24 hex chars)
-- is far too large to brute-force from the prefix alone.
CREATE TABLE api_keys (
                          id SERIAL PRIMARY KEY,
                          name TEXT NOT NULL,
                          key_hash TEXT NOT NULL,
                          prefix TEXT NOT NULL DEFAULT '',
                          last_used TIMESTAMP,
                          create_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          update_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_api_keys_prefix ON api_keys(prefix);

-- Carry the existing single key across so current integrations keep working.
-- Its plaintext is unknown (only the hash was ever stored), so the prefix stays
-- empty; auth falls back to comparing empty-prefix keys and backfills the prefix
-- the first time the key is used.
INSERT INTO api_keys (name, key_hash)
SELECT 'Existing key', value FROM settings WHERE name = 'api_key' AND value != '';

DELETE FROM settings WHERE name = 'api_key';
