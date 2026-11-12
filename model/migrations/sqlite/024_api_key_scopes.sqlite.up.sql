-- Scoped API keys. A key's scopes say what it may do: an access level (read,
-- write or admin, each including the ones below it) and, separately, ingest
-- for devices that push sensor readings. Admin includes everything. Stored as
-- the canonical comma-separated form produced by scope.Set.String, e.g.
-- 'read', 'write,ingest', 'admin'.
--
-- New keys default to read, the safest level. Every key that exists at
-- upgrade time could already do everything, so it becomes admin and nothing
-- that uses it today breaks.
ALTER TABLE api_keys ADD COLUMN scopes TEXT NOT NULL DEFAULT 'read';

UPDATE api_keys SET scopes = 'admin';
