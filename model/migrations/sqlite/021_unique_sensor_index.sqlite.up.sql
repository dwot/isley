-- Migration 021: dedup step 5 of 5 — enforce one sensor per
-- (source, device, type) tuple going forward. Migrations 017-020 have
-- already collapsed any pre-existing duplicates, so this index can be
-- applied cleanly. handlers/sensors.go uses this index for an atomic
-- INSERT ... ON CONFLICT (source, device, type) DO NOTHING upsert.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sensors_source_device_type
    ON sensors(source, device, type);
