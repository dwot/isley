-- Breeder names are unique ignoring case, but older versions allowed
-- duplicates ("Mephisto", "mephisto ", ...). Merge each group into its
-- oldest breeder: its strains move there, it takes a CannaDB link from a
-- duplicate if it has none, and the duplicates go. Then a unique index
-- keeps it that way.
CREATE TEMP TABLE breeder_merge AS
SELECT b.id AS dup_id,
       (SELECT MIN(k.id) FROM breeder k WHERE LOWER(TRIM(k.name)) = LOWER(TRIM(b.name))) AS keep_id
FROM breeder b;
DELETE FROM breeder_merge WHERE dup_id = keep_id;

UPDATE strain
SET breeder_id = (SELECT keep_id FROM breeder_merge WHERE dup_id = strain.breeder_id)
WHERE breeder_id IN (SELECT dup_id FROM breeder_merge);

CREATE TEMP TABLE breeder_link AS
SELECT l.keep_id, l.uri, (SELECT cannadb_indexed_at FROM breeder WHERE cannadb_uri = l.uri) AS indexed_at
FROM (SELECT m.keep_id, MIN(d.cannadb_uri) AS uri
      FROM breeder_merge m
      JOIN breeder d ON d.id = m.dup_id
      JOIN breeder k ON k.id = m.keep_id
      WHERE k.cannadb_uri IS NULL AND d.cannadb_uri IS NOT NULL
      GROUP BY m.keep_id) l;

DELETE FROM breeder WHERE id IN (SELECT dup_id FROM breeder_merge);

UPDATE breeder
SET cannadb_uri = (SELECT uri FROM breeder_link WHERE keep_id = breeder.id),
    cannadb_indexed_at = (SELECT indexed_at FROM breeder_link WHERE keep_id = breeder.id)
WHERE id IN (SELECT keep_id FROM breeder_link);

DROP TABLE breeder_link;
DROP TABLE breeder_merge;

CREATE UNIQUE INDEX idx_breeder_name_lower ON breeder (LOWER(name));
