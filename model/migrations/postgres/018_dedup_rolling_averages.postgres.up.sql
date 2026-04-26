-- Migration 018: dedup step 2 of 5 — drop rolling_averages cache rows
-- for duplicate sensors.
DELETE FROM rolling_averages
WHERE sensor_id IN (
    SELECT id FROM sensors WHERE id NOT IN (
        SELECT MIN(id) FROM sensors GROUP BY source, device, type
    )
);
