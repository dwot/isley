-- The rolling-average trigger computed a window over the sensor's whole
-- history for every inserted reading, so each reading cost more as history
-- grew (and a restore was quadratic). Same result, the way Postgres does it:
-- the 16 readings before the newest, read off the (sensor_id, create_dt)
-- index.
DROP TRIGGER IF EXISTS update_rolling_avg;

CREATE TRIGGER update_rolling_avg
    AFTER INSERT ON sensor_data
BEGIN
    INSERT OR REPLACE INTO rolling_averages (sensor_id, avg_value, create_dt)
    SELECT NEW.sensor_id,
           (SELECT AVG(value) FROM (
                SELECT value FROM sensor_data
                WHERE sensor_id = NEW.sensor_id
                ORDER BY create_dt DESC
                LIMIT 16 OFFSET 1)),
           NEW.create_dt;
END;
