-- Revert lowercase url column back to uppercase URL.
CREATE TABLE streams_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    URL TEXT NOT NULL,
    zone_id INTEGER NOT NULL,
    visible BOOLEAN NOT NULL DEFAULT TRUE,
    FOREIGN KEY (zone_id) REFERENCES zones(id)
);

INSERT INTO streams_new (id, name, URL, zone_id, visible)
    SELECT id, name, url, zone_id, visible FROM streams;

DROP TABLE streams;

ALTER TABLE streams_new RENAME TO streams;
