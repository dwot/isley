ALTER TABLE plant ADD COLUMN sensors TEXT NOT NULL DEFAULT '[]';

UPDATE plant SET sensors = COALESCE((
    SELECT json_agg(sensor_id ORDER BY position, sensor_id)::text
    FROM plant_sensor WHERE plant_id = plant.id
), '[]');

DROP TABLE plant_sensor;
