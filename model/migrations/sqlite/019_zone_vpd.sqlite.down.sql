-- Requires SQLite >= 3.35 (ALTER TABLE DROP COLUMN); satisfied by the bundled
-- modernc.org/sqlite (SQLite 3.50+). A table rebuild is intentionally avoided
-- here because zones is referenced by FKs (sensors.zone_id, plant.zone_id).
ALTER TABLE plant_status DROP COLUMN vpd_high;
ALTER TABLE plant_status DROP COLUMN vpd_low;

ALTER TABLE zones DROP COLUMN vpd_humidity_sensor_id;
ALTER TABLE zones DROP COLUMN vpd_temp_sensor_id;
ALTER TABLE zones DROP COLUMN leaf_temp_offset;
