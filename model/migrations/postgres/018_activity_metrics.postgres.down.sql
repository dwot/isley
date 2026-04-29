DROP INDEX IF EXISTS idx_plant_measurements_activity_id;
ALTER TABLE plant_measurements DROP COLUMN IF EXISTS plant_activity_id;
DROP TABLE IF EXISTS activity_metric;
