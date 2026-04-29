DROP INDEX IF EXISTS idx_plant_measurements_activity_id;

-- SQLite does not support DROP COLUMN before 3.35.0;
-- recreate plant_measurements without plant_activity_id.
CREATE TABLE plant_measurements_backup AS SELECT id, plant_id, metric_id, value, date FROM plant_measurements;
DROP TABLE plant_measurements;
CREATE TABLE plant_measurements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plant_id INTEGER NOT NULL,
    metric_id INTEGER NOT NULL,
    value REAL NOT NULL,
    date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (plant_id) REFERENCES plant(id),
    FOREIGN KEY (metric_id) REFERENCES metric(id)
);
INSERT INTO plant_measurements (id, plant_id, metric_id, value, date)
    SELECT id, plant_id, metric_id, value, date FROM plant_measurements_backup;
DROP TABLE plant_measurements_backup;

DROP TABLE IF EXISTS activity_metric;
