-- Recreate plant table without the parent_plant_id foreign key constraint
CREATE TABLE plant_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    clone INTEGER NOT NULL,
    strain_id INTEGER NOT NULL,
    zone_id INTEGER,
    start_dt DATETIME,
    sensors TEXT NOT NULL DEFAULT '[]',
    harvest_weight DECIMAL(10,2) DEFAULT 0,
    parent_plant_id INT,
    FOREIGN KEY (strain_id) REFERENCES strain(id),
    FOREIGN KEY (zone_id) REFERENCES zones(id) ON DELETE SET NULL
);

INSERT INTO plant_old SELECT id, name, description, clone, strain_id, zone_id, start_dt, sensors, harvest_weight, parent_plant_id FROM plant;

DROP TABLE plant;
ALTER TABLE plant_old RENAME TO plant;
