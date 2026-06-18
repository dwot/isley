ALTER TABLE plant_status DROP COLUMN IF EXISTS vpd_high;
ALTER TABLE plant_status DROP COLUMN IF EXISTS vpd_low;

ALTER TABLE zones DROP COLUMN IF EXISTS vpd_humidity_sensor_id;
ALTER TABLE zones DROP COLUMN IF EXISTS vpd_temp_sensor_id;
ALTER TABLE zones DROP COLUMN IF EXISTS leaf_temp_offset;
