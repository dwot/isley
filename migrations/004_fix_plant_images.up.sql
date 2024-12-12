--Reverse Add plant_images table with plant_id, image_path, image_description, image_order, image_date, created_at, updated_at
DROP TABLE plant_images;

CREATE TABLE plant_images (
                              id INTEGER PRIMARY KEY AUTOINCREMENT,
                              plant_id INT NOT NULL,
                              image_path VARCHAR(255) NOT NULL,
                              image_description TEXT,
                              image_order INT NOT NULL DEFAULT 0,
                              image_date DATE,
                              created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                              updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
