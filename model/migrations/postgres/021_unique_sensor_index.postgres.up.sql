-- Migration 021: dedup step 5 of 5 — enforce unique sensor identity.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sensors_source_device_type
    ON sensors(source, device, type);
