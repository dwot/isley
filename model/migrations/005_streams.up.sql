CREATE TABLE streams (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        name TEXT NOT NULL,
                        URL TEXT NOT NULL,
                        zone_id INTEGER NOT NULL,
                        visible BOOLEAN NOT NULL DEFAULT TRUE,
                        FOREIGN KEY (zone_id) REFERENCES zones(id)
);




