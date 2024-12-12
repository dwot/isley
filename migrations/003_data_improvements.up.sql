--Add sensor unit to sensors table
ALTER TABLE sensors ADD COLUMN unit VARCHAR(255) NOT NULL DEFAULT 'units';

--Remove Seed status from plant_status table
DELETE FROM plant_status WHERE status = 'Seed';

--Add seed_count to strain table
ALTER TABLE strain ADD COLUMN seed_count INT NOT NULL DEFAULT 0;

--Drop status_id from the plant table
ALTER TABLE plant DROP COLUMN status_id;

--Add sensor column to plant table for storing a serialized list of sensor ids in a text column
ALTER TABLE plant ADD COLUMN sensors TEXT NOT NULL DEFAULT '[]';

--Add plant_images table with plant_id, image_path, image_description, image_order, image_date, created_at, updated_at
CREATE TABLE plant_images (
    id SERIAL PRIMARY KEY,
    plant_id INT NOT NULL,
    image_path VARCHAR(255) NOT NULL,
    image_description TEXT,
    image_order INT NOT NULL DEFAULT 0,
    image_date DATE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

