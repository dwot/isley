CREATE TABLE streams (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        name TEXT NOT NULL,
                        URL TEXT NOT NULL,
                        zone_id INTEGER NOT NULL,
                        capture_interval INTEGER NOT NULL,
                        FOREIGN KEY (zone_id) REFERENCES zones(id)
);




