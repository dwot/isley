-- idx_sensor_data_sensor_id is a strict prefix of idx_sensor_data_sensor_id_create_dt
-- (014) and idx_sensor_data_hourly_bucket duplicates sensor_data_hourly's primary
-- key (015). Neither can win a plan the covering index loses, and together they
-- cost ~100MB on a 4.6M-row instance plus maintenance on every insert.
DROP INDEX IF EXISTS idx_sensor_data_sensor_id;
DROP INDEX IF EXISTS idx_sensor_data_hourly_bucket;
