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

-- Create the sensor table
CREATE TABLE sensors (
                         id INTEGER PRIMARY KEY AUTOINCREMENT,
                         name TEXT NOT NULL,
                         zone_id INTEGER NOT NULL,
                         source TEXT NOT NULL, -- integration type i.e. acinfinity, ecowitt, etc
                         device TEXT NOT NULL, -- device unique id from source
                         type TEXT NOT NULL, -- sensor type i.e. temperature, humidity, etc
                         create_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         update_dt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         FOREIGN KEY (zone_id) REFERENCES zones(id)
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
                        breeder TEXT NOT NULL,
                        sativa INTEGER NOT NULL,
                        indica INTEGER NOT NULL,
                        autoflower INTEGER NOT NULL,
                        description TEXT NOT NULL
);

-- Create the plant_status table
CREATE TABLE plant_status (
                              id INTEGER PRIMARY KEY AUTOINCREMENT,
                              status TEXT NOT NULL,
                              active INTEGER NOT NULL
);

-- Load the plant_status table
INSERT INTO plant_status (status, active) VALUES ('Seed', 0);
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
                       status_id INTEGER NOT NULL,
                       description TEXT NOT NULL,
                       clone INTEGER NOT NULL,
                       strain_id INTEGER NOT NULL,
                       zone_id INTEGER NOT NULL,
                       start_dt DATETIME,
                       FOREIGN KEY (strain_id) REFERENCES strain(id),
                       FOREIGN KEY (zone_id) REFERENCES zones(id)
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

-- Create the plant_note table
CREATE TABLE plant_note (
                            id INTEGER PRIMARY KEY AUTOINCREMENT,
                            plant_id INTEGER NOT NULL,
                            note TEXT NOT NULL,
                            date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                            FOREIGN KEY (plant_id) REFERENCES plant(id)
);

-- Create the metric table
CREATE TABLE metric (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        name TEXT NOT NULL,
                        unit TEXT NOT NULL
);

-- Load the metric table
INSERT INTO metric (name, unit) VALUES ('Temperature', 'F');
INSERT INTO metric (name, unit) VALUES ('Humidity', '%');
INSERT INTO metric (name, unit) VALUES ('Light Intensity', 'PPFD');
INSERT INTO metric (name, unit) VALUES ('Height', 'in');

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
                          name TEXT NOT NULL
);

-- Load the activity table
INSERT INTO activity (name) VALUES ('Water');
INSERT INTO activity (name) VALUES ('Feed');
INSERT INTO activity (name) VALUES ('Trim');
INSERT INTO activity (name) VALUES ('Transplant');
INSERT INTO activity (name) VALUES ('Clone');
INSERT INTO activity (name) VALUES ('Light Change');
INSERT INTO activity (name) VALUES ('Environment Change');
INSERT INTO activity (name) VALUES ('Train');

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

