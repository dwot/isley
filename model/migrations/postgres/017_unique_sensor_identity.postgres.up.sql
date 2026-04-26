-- Migration 017: enforce unique sensor identity by (source, device, type).
-- See sqlite/017_unique_sensor_identity for rationale.

CREATE TEMP TABLE _sensor_dedup_map AS
    SELECT
        s.id AS dup_id,
        (SELECT MIN(id) FROM sensors
            WHERE source = s.source AND device = s.device AND type = s.type) AS keep_id
    FROM sensors s
    WHERE s.id <> (SELECT MIN(id) FROM sensors
            WHERE source = s.source AND device = s.device AND type = s.type);

UPDATE sensor_data
    SET sensor_id = m.keep_id
    FROM _sensor_dedup_map m
    WHERE sensor_data.sensor_id = m.dup_id;

DELETE FROM rolling_averages
    WHERE sensor_id IN (SELECT dup_id FROM _sensor_dedup_map);

DELETE FROM sensor_data_hourly
    WHERE sensor_id IN (SELECT dup_id FROM _sensor_dedup_map);

DELETE FROM sensors
    WHERE id IN (SELECT dup_id FROM _sensor_dedup_map);

DROP TABLE _sensor_dedup_map;

CREATE UNIQUE INDEX IF NOT EXISTS idx_sensors_source_device_type
    ON sensors(source, device, type);
