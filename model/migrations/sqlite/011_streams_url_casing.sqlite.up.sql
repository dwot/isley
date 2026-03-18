-- Rename uppercase URL column to lowercase url for consistency with PostgreSQL.
-- SQLite does not support ALTER TABLE RENAME COLUMN prior to 3.25.0 (2018),
-- so we recreate the table.
CREATE TABLE streams_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    zone_id INTEGER NOT NULL,
    visible BOOLEAN NOT NULL DEFAULT TRUE,
    FOREIGN KEY (zone_id) REFERENCES zones(id)
);

INSERT INTO streams_new (id, name, url, zone_id, visible)
    SELECT id, name, URL, zone_id, visible FROM streams;

DROP TABLE streams;

ALTER TABLE streams_new RENAME TO streams;
