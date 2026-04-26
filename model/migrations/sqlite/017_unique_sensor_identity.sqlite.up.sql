-- Migration 017: enforce unique sensor identity by (source, device, type).
--
-- Background: handlers/sensors.go IngestSensorData performed a SELECT
-- followed by a conditional INSERT, which under concurrent ingest
-- created duplicate sensor rows. The application layer now uses an
-- atomic upsert (INSERT ... ON CONFLICT ... DO NOTHING + re-SELECT)
-- that depends on the unique index this migration adds.
--
-- Step 1: collapse any pre-existing duplicate sensors. For each
-- (source, device, type) tuple with multiple rows, the row with the
-- lowest id is kept; sensor_data rows are repointed to the canonical
-- id; rolling_averages and sensor_data_hourly rows for duplicates are
-- dropped (both are job/trigger-maintained caches that refresh on
-- their own).

CREATE TEMP TABLE _sensor_dedup_map AS
    SELECT
        s.id AS dup_id,
        (SELECT MIN(id) FROM sensors
            WHERE source = s.source AND device = s.device AND type = s.type) AS keep_id
    FROM sensors s
    WHERE s.id != (SELECT MIN(id) FROM sensors
            WHERE source = s.source AND device = s.device AND type = s.type);

UPDATE sensor_data
    SET sensor_id = (SELECT keep_id FROM _sensor_dedup_map
                     WHERE dup_id = sensor_data.sensor_id)
    WHERE sensor_id IN (SELECT dup_id FROM _sensor_dedup_map);

DELETE FROM rolling_averages
    WHERE sensor_id IN (SELECT dup_id FROM _sensor_dedup_map);

DELETE FROM sensor_data_hourly
    WHERE sensor_id IN (SELECT dup_id FROM _sensor_dedup_map);

DELETE FROM sensors
    WHERE id IN (SELECT dup_id FROM _sensor_dedup_map);

DROP TABLE _sensor_dedup_map;

-- Step 2: enforce uniqueness so the application's ON CONFLICT path works.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sensors_source_device_type
    ON sensors(source, device, type);
