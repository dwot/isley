-- Add composite index for chart queries that filter on (sensor_id, create_dt)
CREATE INDEX IF NOT EXISTS idx_sensor_data_sensor_id_create_dt ON sensor_data(sensor_id, create_dt);
