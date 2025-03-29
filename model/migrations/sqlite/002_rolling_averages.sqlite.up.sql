CREATE TABLE rolling_averages (
                                  sensor_id INTEGER,
                                  avg_value REAL,
                                  create_dt DATETIME,
                                  PRIMARY KEY(sensor_id)
);

CREATE TRIGGER update_rolling_avg
    AFTER INSERT ON sensor_data
BEGIN
    INSERT OR REPLACE INTO rolling_averages(sensor_id, avg_value, create_dt)
    SELECT
        sd.sensor_id,
        AVG(sd.value) OVER (
            PARTITION BY sd.sensor_id
            ORDER BY sd.create_dt ASC
            ROWS BETWEEN 16 PRECEDING AND 1 PRECEDING
        ),
            NEW.create_dt
    FROM sensor_data sd
    WHERE sd.sensor_id = NEW.sensor_id;
END;
