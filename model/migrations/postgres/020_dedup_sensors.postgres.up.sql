-- Migration 020: dedup step 4 of 5 — delete duplicate sensor rows.
DELETE FROM sensors
WHERE id NOT IN (
    SELECT MIN(id) FROM sensors GROUP BY source, device, type
);
