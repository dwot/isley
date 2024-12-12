--Back out script for 003_data_improvements.up.sql
--Reverse Add sensor unit to sensors table
ALTER TABLE sensors DROP COLUMN unit;

--Reverse Remove Seed status from plant_status table
INSERT INTO plant_status (status, active) VALUES ('Seed', 0);

--Reverse Add seed_count to strain table
ALTER TABLE strain DROP COLUMN seed_count;

--Reverse Drop status_id from the plant table
ALTER TABLE plant ADD COLUMN status_id INT NOT NULL;

--Reverse Add sensor column to plant table for storing a serialized list of sensor ids in a text column
ALTER TABLE plant DROP COLUMN sensors;

--Reverse Add plant_images table with plant_id, image_path, image_description, image_order, image_date, created_at, updated_at
DROP TABLE plant_images;
