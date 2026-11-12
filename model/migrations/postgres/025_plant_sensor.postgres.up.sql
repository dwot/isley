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

INSERT INTO plant_sensor (plant_id, sensor_id, position)
SELECT p.id, j.value::int, (j.ord - 1)::int
FROM plant p
CROSS JOIN LATERAL json_array_elements_text(p.sensors::json) WITH ORDINALITY AS j(value, ord)
WHERE p.sensors ~ '^\s*\['
  AND j.value ~ '^\d+$'
  AND j.value::int IN (SELECT id FROM sensors)
ON CONFLICT (plant_id, sensor_id) DO NOTHING;

ALTER TABLE plant DROP COLUMN sensors;
