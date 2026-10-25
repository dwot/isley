CREATE INDEX IF NOT EXISTS idx_sensor_data_sensor_id ON sensor_data(sensor_id);
CREATE INDEX IF NOT EXISTS idx_sensor_data_hourly_bucket ON sensor_data_hourly(sensor_id, bucket);
