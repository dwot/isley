-- Sensors linked directly to a plant (a soil probe in its pot) move from a
-- JSON array in plant.sensors to a join table. position keeps the order the
-- user linked them in. Links to sensors that no longer exist are dropped.
CREATE TABLE plant_sensor (
    plant_id  INTEGER NOT NULL REFERENCES plant(id) ON DELETE CASCADE,
    sensor_id INTEGER NOT NULL REFERENCES sensors(id) ON DELETE CASCADE,
    position  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (plant_id, sensor_id)
);
CREATE INDEX idx_plant_sensor_sensor ON plant_sensor (sensor_id);

INSERT OR IGNORE INTO plant_sensor (plant_id, sensor_id, position)
SELECT p.id, CAST(j.value AS INTEGER), CAST(j.key AS INTEGER)
FROM plant p, json_each(p.sensors) j
WHERE json_valid(p.sensors)
  AND json_type(p.sensors) = 'array'
  AND CAST(j.value AS INTEGER) IN (SELECT id FROM sensors);

ALTER TABLE plant DROP COLUMN sensors;
