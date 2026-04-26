-- Migration 019: dedup step 3 of 5 — drop sensor_data_hourly cache
-- rows for duplicate sensors.
DELETE FROM sensor_data_hourly
WHERE sensor_id IN (
    SELECT id FROM sensors WHERE id NOT IN (
        SELECT MIN(id) FROM sensors GROUP BY source, device, type
    )
);
