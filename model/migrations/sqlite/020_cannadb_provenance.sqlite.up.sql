-- Provenance for strains/breeders imported from CannaDB (api.cannadb.net).
-- cannadb_uri is the record's AT-URI and doubles as the dedupe/upsert key so a
-- re-import updates in place instead of creating a duplicate row.
-- cannadb_indexed_at stores the record's indexedAt for future refresh logic.
ALTER TABLE strain ADD COLUMN cannadb_uri TEXT;
ALTER TABLE strain ADD COLUMN cannadb_indexed_at TEXT;
ALTER TABLE breeder ADD COLUMN cannadb_uri TEXT;
ALTER TABLE breeder ADD COLUMN cannadb_indexed_at TEXT;

-- Partial-unique so manually-created rows (NULL uri) are unconstrained while
-- imported rows are unique per CannaDB record.
CREATE UNIQUE INDEX idx_strain_cannadb_uri ON strain(cannadb_uri) WHERE cannadb_uri IS NOT NULL;
CREATE UNIQUE INDEX idx_breeder_cannadb_uri ON breeder(cannadb_uri) WHERE cannadb_uri IS NOT NULL;
