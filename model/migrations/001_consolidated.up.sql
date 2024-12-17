-- Create the settings table
CREATE TABLE settings (
                          id INTEGER PRIMARY KEY AUTOINCREMENT,
                          name TEXT NOT NULL,
                          value TEXT NOT NULL,
                          create_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          update_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Trigger to set create_dt on insert
CREATE TRIGGER trg_settings_create_dt
    AFTER INSERT ON settings
    FOR EACH ROW
BEGIN
    UPDATE settings
    SET create_dt = CURRENT_TIMESTAMP,
        update_dt = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;

-- Trigger to set update_dt on insert or update
CREATE TRIGGER trg_settings_update_dt
    AFTER UPDATE ON settings
    FOR EACH ROW
BEGIN
    UPDATE settings
    SET update_dt = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;


-- Create the zones table
CREATE TABLE zones (
                       id INTEGER PRIMARY KEY AUTOINCREMENT,
                       name TEXT NOT NULL,
                       create_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       update_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Trigger to set create_dt on insert
CREATE TRIGGER trg_zones_create_dt
    AFTER INSERT ON zones
    FOR EACH ROW
BEGIN
    UPDATE zones
    SET create_dt = CURRENT_TIMESTAMP,
        update_dt = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;

-- Trigger to set update_dt on insert or update
CREATE TRIGGER trg_zones_update_dt
    AFTER UPDATE ON zones
    FOR EACH ROW
BEGIN
    UPDATE zones
    SET update_dt = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;


-- Create the sensor table with optional zone_id
CREATE TABLE sensors (
                         id INTEGER PRIMARY KEY AUTOINCREMENT,
                         name TEXT NOT NULL,
                         zone_id INTEGER, -- Make zone_id optional (NULL allowed)
                         source TEXT NOT NULL, -- integration type i.e. acinfinity, ecowitt, etc
                         device TEXT NOT NULL, -- device unique id from source
                         type TEXT NOT NULL, -- sensor type i.e. temperature, humidity, etc
                         create_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         update_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         show BOOLEAN NOT NULL DEFAULT TRUE,
                         unit VARCHAR(255) NOT NULL DEFAULT 'units',
                         FOREIGN KEY (zone_id) REFERENCES zones(id) ON DELETE SET NULL
);
-- Trigger to set create_dt on insert
CREATE TRIGGER trg_sensors_create_dt
    AFTER INSERT ON sensors
    FOR EACH ROW
BEGIN
    UPDATE sensors
    SET create_dt = CURRENT_TIMESTAMP,
        update_dt = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;

-- Trigger to set update_dt on insert or update
CREATE TRIGGER trg_sensors_update_dt
    AFTER UPDATE ON sensors
    FOR EACH ROW
BEGIN
    UPDATE sensors
    SET update_dt = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;

-- Create the sensor_data table
CREATE TABLE sensor_data (
                             id INTEGER PRIMARY KEY AUTOINCREMENT,
                             sensor_id INTEGER NOT NULL,
                             value REAL NOT NULL,
                             create_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                             FOREIGN KEY (sensor_id) REFERENCES sensors(id)
);

-- Create the strain table
CREATE TABLE strain (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        name TEXT NOT NULL,
                        sativa INTEGER NOT NULL,
                        indica INTEGER NOT NULL,
                        autoflower INTEGER NOT NULL,
                        description TEXT NOT NULL,
                        seed_count INT NOT NULL DEFAULT 0,
                        breeder_id INTEGER NOT NULL,
                        FOREIGN KEY (breeder_id) REFERENCES breeder(id)
);

-- Create the plant_status table
CREATE TABLE plant_status (
                              id INTEGER PRIMARY KEY AUTOINCREMENT,
                              status TEXT NOT NULL,
                              active INTEGER NOT NULL
);


INSERT INTO plant_status (status, active) VALUES ('Seedling', 1);
INSERT INTO plant_status (status, active) VALUES ('Veg', 1);
INSERT INTO plant_status (status, active) VALUES ('Flower', 1);
INSERT INTO plant_status (status, active) VALUES ('Drying', 1);
INSERT INTO plant_status (status, active) VALUES ('Curing', 1);
INSERT INTO plant_status (status, active) VALUES ('Success', 0);
INSERT INTO plant_status (status, active) VALUES ('Dead', 0);

-- Create the plant table
CREATE TABLE plant (
                       id INTEGER PRIMARY KEY AUTOINCREMENT,
                       name TEXT NOT NULL,
                       description TEXT NOT NULL,
                       clone INTEGER NOT NULL,
                       strain_id INTEGER NOT NULL,
                       zone_id INTEGER,
                       start_dt DATETIME,
                       sensors TEXT NOT NULL DEFAULT '[]',
                       harvest_weight DECIMAL(10,2) DEFAULT 0,
                       FOREIGN KEY (strain_id) REFERENCES strain(id),
                       FOREIGN KEY (zone_id) REFERENCES zones(id) ON DELETE SET NULL
);


-- Create the plant_status_log table
CREATE TABLE plant_status_log (
                                  id INTEGER PRIMARY KEY AUTOINCREMENT,
                                  plant_id INTEGER NOT NULL,
                                  status_id INTEGER NOT NULL,
                                  date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                  FOREIGN KEY (plant_id) REFERENCES plant(id),
                                  FOREIGN KEY (status_id) REFERENCES plant_status(id)
);


-- Create the metric table
CREATE TABLE metric (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        name TEXT NOT NULL,
                        unit TEXT NOT NULL,
                        lock BOOLEAN DEFAULT FALSE
);

-- Load the metric table
INSERT INTO metric (name, unit, lock) VALUES ('Height', 'in', TRUE);


-- Create the plant_measurements table
CREATE TABLE plant_measurements (
                                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                                    plant_id INTEGER NOT NULL,
                                    metric_id INTEGER NOT NULL,
                                    value REAL NOT NULL,
                                    date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                    FOREIGN KEY (plant_id) REFERENCES plant(id),
                                    FOREIGN KEY (metric_id) REFERENCES metric(id)
);

-- Create the activity table
CREATE TABLE activity (
                          id INTEGER PRIMARY KEY AUTOINCREMENT,
                          name TEXT NOT NULL,
                          lock BOOLEAN DEFAULT FALSE
);

-- Load the activity table
INSERT INTO activity (name, lock) VALUES ('Water', TRUE);
INSERT INTO activity (name, lock) VALUES ('Feed', TRUE);
INSERT INTO activity (name, lock) VALUES ('Note', TRUE);

-- Create the plant_activity table
CREATE TABLE plant_activity (
                                id INTEGER PRIMARY KEY AUTOINCREMENT,
                                plant_id INTEGER NOT NULL,
                                activity_id INTEGER NOT NULL,
                                date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                note TEXT NOT NULL,
                                FOREIGN KEY (plant_id) REFERENCES plant(id),
                                FOREIGN KEY (activity_id) REFERENCES activity(id)
);

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

CREATE TABLE breeder (
                         id INTEGER PRIMARY KEY AUTOINCREMENT,
                         name VARCHAR(255) NOT NULL
);

INSERT INTO settings (name, value) VALUES ('polling_interval', '60');