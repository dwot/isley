-- Migration 020: dedup step 4 of 5 — delete the duplicate sensor rows
-- themselves now that nothing references them. The canonical (lowest
-- id) row for each (source, device, type) tuple survives.
DELETE FROM sensors
WHERE id NOT IN (
    SELECT MIN(id) FROM sensors GROUP BY source, device, type
);
