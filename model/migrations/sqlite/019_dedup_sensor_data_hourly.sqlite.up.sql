-- Migration 019: dedup step 3 of 5 — drop sensor_data_hourly rows for
-- non-canonical duplicate sensors. sensor_data_hourly is a job-
-- maintained rollup cache (see watcher/rollup.go RefreshHourlyRollups)
-- that rebuilds from sensor_data on each cycle, so deleting these
-- rows is safe.
DELETE FROM sensor_data_hourly
WHERE sensor_id IN (
    SELECT id FROM sensors WHERE id NOT IN (
        SELECT MIN(id) FROM sensors GROUP BY source, device, type
    )
);
