-- Hourly rollup table for sensor data downsampling
-- Chart queries for ranges >24h read from this table instead of raw sensor_data
CREATE TABLE IF NOT EXISTS sensor_data_hourly (
    sensor_id   INTEGER  NOT NULL,
    bucket      DATETIME NOT NULL,
    min_val     REAL     NOT NULL,
    max_val     REAL     NOT NULL,
    avg_val     REAL     NOT NULL,
    sample_count INTEGER NOT NULL,
    PRIMARY KEY (sensor_id, bucket),
    FOREIGN KEY (sensor_id) REFERENCES sensors(id)
);

CREATE INDEX IF NOT EXISTS idx_sensor_data_hourly_bucket ON sensor_data_hourly(sensor_id, bucket);
