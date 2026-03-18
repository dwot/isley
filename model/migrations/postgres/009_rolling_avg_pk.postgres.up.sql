-- Align PostgreSQL rolling_averages with SQLite: one row per sensor.
-- Historical rolling averages are never queried; only the latest matters for trend arrows.

-- 1. Drop the existing trigger that inserts historical rows
DROP TRIGGER IF EXISTS update_rolling_avg ON sensor_data;
DROP FUNCTION IF EXISTS fn_update_rolling_avg();

-- 2. Drop the old table (historical data is disposable cache)
DROP TABLE rolling_averages;

-- 3. Recreate with single-column primary key and no old data.
--    The trigger will repopulate one row per sensor on the next sensor_data insert.
CREATE TABLE rolling_averages (
    sensor_id INTEGER PRIMARY KEY,
    avg_value REAL,
    create_dt TIMESTAMP
);

-- 5. Recreate trigger with upsert on sensor_id only
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
    ON CONFLICT (sensor_id) DO UPDATE
        SET avg_value = EXCLUDED.avg_value,
            create_dt = EXCLUDED.create_dt;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_rolling_avg
    AFTER INSERT ON sensor_data
    FOR EACH ROW
EXECUTE FUNCTION fn_update_rolling_avg();
