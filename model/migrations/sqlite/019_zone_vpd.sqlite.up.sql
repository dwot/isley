-- Per-zone VPD configuration: leaf-temp offset (also the on/off switch) and the
-- temperature + humidity sensors selected as the VPD inputs for the zone.
ALTER TABLE zones ADD COLUMN leaf_temp_offset REAL;
ALTER TABLE zones ADD COLUMN vpd_temp_sensor_id INTEGER REFERENCES sensors(id) ON DELETE SET NULL;
ALTER TABLE zones ADD COLUMN vpd_humidity_sensor_id INTEGER REFERENCES sensors(id) ON DELETE SET NULL;

-- Per-growth-stage ideal VPD range (kPa). NULL = no indicator for that stage.
ALTER TABLE plant_status ADD COLUMN vpd_low REAL;
ALTER TABLE plant_status ADD COLUMN vpd_high REAL;

-- Seed defaults for the active growing stages (sources: Humboldt Seed Co., Smartfog).
UPDATE plant_status SET vpd_low = 0.4, vpd_high = 0.8 WHERE status = 'Seedling';
UPDATE plant_status SET vpd_low = 0.8, vpd_high = 1.2 WHERE status = 'Veg';
UPDATE plant_status SET vpd_low = 1.2, vpd_high = 1.5 WHERE status = 'Flower';
