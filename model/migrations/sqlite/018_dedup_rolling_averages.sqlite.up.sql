-- Migration 018: dedup step 2 of 5 — drop rolling_averages cache rows
-- belonging to non-canonical duplicate sensors. rolling_averages is a
-- trigger-maintained cache and refreshes on subsequent writes, so
-- deleting these rows is safe.
DELETE FROM rolling_averages
WHERE sensor_id IN (
    SELECT id FROM sensors WHERE id NOT IN (
        SELECT MIN(id) FROM sensors GROUP BY source, device, type
    )
);
