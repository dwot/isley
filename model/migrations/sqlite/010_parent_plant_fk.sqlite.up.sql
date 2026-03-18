-- Clean up invalid parent_plant_id values before adding the constraint.
-- Rows with 0 or referencing non-existent plants should be NULL (no parent).
UPDATE plant SET parent_plant_id = NULL
    WHERE parent_plant_id IS NOT NULL
      AND parent_plant_id NOT IN (SELECT id FROM plant);

-- SQLite does not support ALTER TABLE ADD CONSTRAINT for foreign keys.
-- To add a FK on parent_plant_id we must recreate the plant table.
-- This preserves all existing data while adding the constraint.

CREATE TABLE plant_new (
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
    FOREIGN KEY (zone_id) REFERENCES zones(id) ON DELETE SET NULL,
    FOREIGN KEY (parent_plant_id) REFERENCES plant(id) ON DELETE SET NULL
);

INSERT INTO plant_new SELECT id, name, description, clone, strain_id, zone_id, start_dt, sensors, harvest_weight, parent_plant_id FROM plant;

DROP TABLE plant;
ALTER TABLE plant_new RENAME TO plant;
