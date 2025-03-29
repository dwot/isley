-- Create the settings table
CREATE TABLE settings (
                          id SERIAL PRIMARY KEY,
                          name TEXT NOT NULL,
                          value TEXT NOT NULL,
                          create_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          update_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Trigger to set timestamps on insert
CREATE OR REPLACE FUNCTION trg_settings_create_dt() RETURNS TRIGGER AS $$
BEGIN
    NEW.create_dt := CURRENT_TIMESTAMP;
NEW.update_dt := CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_settings_create_dt
    BEFORE INSERT ON settings
    FOR EACH ROW
    EXECUTE FUNCTION trg_settings_create_dt();

-- Trigger to update timestamp on update
CREATE OR REPLACE FUNCTION trg_settings_update_dt() RETURNS TRIGGER AS $$
BEGIN
    NEW.update_dt := CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_settings_update_dt
    BEFORE UPDATE ON settings
    FOR EACH ROW
    EXECUTE FUNCTION trg_settings_update_dt();

-- Create the zones table
CREATE TABLE zones (
                       id SERIAL PRIMARY KEY,
                       name TEXT NOT NULL,
                       create_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       update_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE OR REPLACE FUNCTION trg_zones_create_dt() RETURNS TRIGGER AS $$
BEGIN
    NEW.create_dt := CURRENT_TIMESTAMP;
NEW.update_dt := CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_zones_create_dt
    BEFORE INSERT ON zones
    FOR EACH ROW
    EXECUTE FUNCTION trg_zones_create_dt();

CREATE OR REPLACE FUNCTION trg_zones_update_dt() RETURNS TRIGGER AS $$
BEGIN
    NEW.update_dt := CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_zones_update_dt
    BEFORE UPDATE ON zones
    FOR EACH ROW
    EXECUTE FUNCTION trg_zones_update_dt();

-- Create the breeder table
CREATE TABLE breeder (
                         id SERIAL PRIMARY KEY,
                         name VARCHAR(255) NOT NULL
);

-- Create the sensor table
CREATE TABLE sensors (
                         id SERIAL PRIMARY KEY,
                         name TEXT NOT NULL,
                         zone_id INTEGER REFERENCES zones(id) ON DELETE SET NULL,
                         source TEXT NOT NULL,
                         device TEXT NOT NULL,
                         type TEXT NOT NULL,
                         create_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         update_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         show BOOLEAN NOT NULL DEFAULT TRUE,
                         unit VARCHAR(255) NOT NULL DEFAULT 'units'
);

CREATE OR REPLACE FUNCTION trg_sensors_create_dt() RETURNS TRIGGER AS $$
BEGIN
    NEW.create_dt := CURRENT_TIMESTAMP;
NEW.update_dt := CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sensors_create_dt
    BEFORE INSERT ON sensors
    FOR EACH ROW
    EXECUTE FUNCTION trg_sensors_create_dt();

CREATE OR REPLACE FUNCTION trg_sensors_update_dt() RETURNS TRIGGER AS $$
BEGIN
    NEW.update_dt := CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sensors_update_dt
    BEFORE UPDATE ON sensors
    FOR EACH ROW
    EXECUTE FUNCTION trg_sensors_update_dt();

-- Create the sensor_data table
CREATE TABLE sensor_data (
                             id SERIAL PRIMARY KEY,
                             sensor_id INTEGER NOT NULL REFERENCES sensors(id),
                             value REAL NOT NULL,
                             create_dt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create the strain table
CREATE TABLE strain (
                        id SERIAL PRIMARY KEY,
                        name TEXT NOT NULL,
                        sativa INTEGER NOT NULL,
                        indica INTEGER NOT NULL,
                        autoflower INTEGER NOT NULL,
                        description TEXT NOT NULL,
                        seed_count INTEGER NOT NULL DEFAULT 0,
                        breeder_id INTEGER NOT NULL REFERENCES breeder(id)
);

-- Create the plant_status table
CREATE TABLE plant_status (
                              id SERIAL PRIMARY KEY,
                              status TEXT NOT NULL,
                              active INTEGER NOT NULL
);

INSERT INTO plant_status (status, active) VALUES
                                              ('Seedling', 1),
                                              ('Veg', 1),
                                              ('Flower', 1),
                                              ('Drying', 1),
                                              ('Curing', 1),
                                              ('Success', 0),
                                              ('Dead', 0);

-- Create the plant table
CREATE TABLE plant (
                       id SERIAL PRIMARY KEY,
                       name TEXT NOT NULL,
                       description TEXT NOT NULL,
                       clone INTEGER NOT NULL,
                       strain_id INTEGER NOT NULL REFERENCES strain(id),
                       zone_id INTEGER REFERENCES zones(id) ON DELETE SET NULL,
                       start_dt TIMESTAMP,
                       sensors TEXT NOT NULL DEFAULT '[]',
                       harvest_weight NUMERIC(10,2) DEFAULT 0
);

-- Create the plant_status_log table
CREATE TABLE plant_status_log (
                                  id SERIAL PRIMARY KEY,
                                  plant_id INTEGER NOT NULL REFERENCES plant(id),
                                  status_id INTEGER NOT NULL REFERENCES plant_status(id),
                                  date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create the metric table
CREATE TABLE metric (
                        id SERIAL PRIMARY KEY,
                        name TEXT NOT NULL,
                        unit TEXT NOT NULL,
                        lock BOOLEAN DEFAULT FALSE
);

INSERT INTO metric (name, unit, lock) VALUES ('Height', 'in', TRUE);

-- Create the plant_measurements table
CREATE TABLE plant_measurements (
                                    id SERIAL PRIMARY KEY,
                                    plant_id INTEGER NOT NULL REFERENCES plant(id),
                                    metric_id INTEGER NOT NULL REFERENCES metric(id),
                                    value REAL NOT NULL,
                                    date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create the activity table
CREATE TABLE activity (
                          id SERIAL PRIMARY KEY,
                          name TEXT NOT NULL,
                          lock BOOLEAN DEFAULT FALSE
);

INSERT INTO activity (name, lock) VALUES
                                      ('Water', TRUE),
                                      ('Feed', TRUE),
                                      ('Note', TRUE);

-- Create the plant_activity table
CREATE TABLE plant_activity (
                                id SERIAL PRIMARY KEY,
                                plant_id INTEGER NOT NULL REFERENCES plant(id),
                                activity_id INTEGER NOT NULL REFERENCES activity(id),
                                date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                note TEXT NOT NULL
);

-- Create the plant_images table
CREATE TABLE plant_images (
                              id SERIAL PRIMARY KEY,
                              plant_id INTEGER NOT NULL REFERENCES plant(id),
                              image_path VARCHAR(255) NOT NULL,
                              image_description TEXT,
                              image_order INTEGER NOT NULL DEFAULT 0,
                              image_date DATE,
                              created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                              updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed settings
INSERT INTO settings (name, value) VALUES ('polling_interval', '60');
