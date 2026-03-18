-- Add indexes for frequently queried columns
CREATE INDEX IF NOT EXISTS idx_sensor_data_sensor_id ON sensor_data(sensor_id);
CREATE INDEX IF NOT EXISTS idx_sensor_data_create_dt ON sensor_data(create_dt);
CREATE INDEX IF NOT EXISTS idx_plant_activity_plant_id ON plant_activity(plant_id);
CREATE INDEX IF NOT EXISTS idx_plant_status_log_plant_id ON plant_status_log(plant_id);
CREATE INDEX IF NOT EXISTS idx_sensors_source_device_type ON sensors(source, device, type);
CREATE INDEX IF NOT EXISTS idx_plant_measurements_plant_id ON plant_measurements(plant_id);
CREATE INDEX IF NOT EXISTS idx_plant_images_plant_id ON plant_images(plant_id);
CREATE INDEX IF NOT EXISTS idx_sensors_zone_id ON sensors(zone_id);
CREATE INDEX IF NOT EXISTS idx_strain_breeder_id ON strain(breeder_id);
CREATE INDEX IF NOT EXISTS idx_settings_name ON settings(name);
