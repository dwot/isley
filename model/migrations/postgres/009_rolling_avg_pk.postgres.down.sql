-- Revert to composite primary key (sensor_id, create_dt)
DROP TRIGGER IF EXISTS update_rolling_avg ON sensor_data;
DROP FUNCTION IF EXISTS fn_update_rolling_avg();

CREATE TABLE rolling_averages_old (
    sensor_id INTEGER,
    avg_value REAL,
    create_dt TIMESTAMP,
    PRIMARY KEY (sensor_id, create_dt)
);

INSERT INTO rolling_averages_old (sensor_id, avg_value, create_dt)
    SELECT sensor_id, avg_value, create_dt FROM rolling_averages;

DROP TABLE rolling_averages;
ALTER TABLE rolling_averages_old RENAME TO rolling_averages;

CREATE OR REPLACE FUNCTION fn_update_rolling_avg()
    RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO rolling_averages (sensor_id, avg_value, create_dt)
    SELECT
        NEW.sensor_id,
        (
            SELECT AVG(value)
            FROM (
                     SELECT value
                     FROM sensor_data
                     WHERE sensor_id = NEW.sensor_id
                     ORDER BY create_dt DESC
                     LIMIT 16 OFFSET 1
                 ) sub
        ),
        NEW.create_dt
    ON CONFLICT (sensor_id, create_dt) DO UPDATE
        SET avg_value = EXCLUDED.avg_value;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_rolling_avg
    AFTER INSERT ON sensor_data
    FOR EACH ROW
EXECUTE FUNCTION fn_update_rolling_avg();
