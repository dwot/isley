-- Migration 017: dedup step 1 of 5 — repoint sensor_data rows from
-- duplicate sensors onto the canonical (lowest id) sensor for each
-- (source, device, type) tuple.
--
-- Background: handlers/sensors.go IngestSensorData previously did a
-- SELECT-then-INSERT, which under concurrent ingest created duplicate
-- sensor rows. The application now uses INSERT ... ON CONFLICT
-- DO NOTHING + re-SELECT, which depends on the unique index added in
-- migration 021. Migrations 017-020 collapse any pre-existing
-- duplicates so that 021's unique index can be applied without
-- conflict.
--
-- One statement per migration file by convention; golang-migrate's
-- multi-statement support varies by driver, so we keep each file
-- single-statement to be portable.

UPDATE sensor_data
SET sensor_id = (
    SELECT MIN(s2.id) FROM sensors s2
    JOIN sensors s_self ON s_self.id = sensor_data.sensor_id
    WHERE s2.source = s_self.source
      AND s2.device = s_self.device
      AND s2.type   = s_self.type
)
WHERE sensor_id IN (
    SELECT id FROM sensors WHERE id NOT IN (
        SELECT MIN(id) FROM sensors GROUP BY source, device, type
    )
);
