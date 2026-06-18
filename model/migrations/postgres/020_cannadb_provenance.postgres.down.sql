DROP INDEX IF EXISTS idx_breeder_cannadb_uri;
DROP INDEX IF EXISTS idx_strain_cannadb_uri;
ALTER TABLE breeder DROP COLUMN IF EXISTS cannadb_indexed_at;
ALTER TABLE breeder DROP COLUMN IF EXISTS cannadb_uri;
ALTER TABLE strain DROP COLUMN IF EXISTS cannadb_indexed_at;
ALTER TABLE strain DROP COLUMN IF EXISTS cannadb_uri;
