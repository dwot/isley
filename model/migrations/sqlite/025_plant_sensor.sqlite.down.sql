ALTER TABLE plant ADD COLUMN sensors TEXT NOT NULL DEFAULT '[]';

UPDATE plant SET sensors = COALESCE((
    SELECT json_group_array(sensor_id) FROM (
        SELECT sensor_id FROM plant_sensor WHERE plant_id = plant.id ORDER BY position, sensor_id
    )
), '[]');

DROP TABLE plant_sensor;
