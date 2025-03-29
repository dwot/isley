CREATE TABLE streams (
                         id SERIAL PRIMARY KEY,
                         name TEXT NOT NULL,
                         url TEXT NOT NULL,
                         zone_id INTEGER NOT NULL REFERENCES zones(id),
                         visible BOOLEAN NOT NULL DEFAULT TRUE
);
