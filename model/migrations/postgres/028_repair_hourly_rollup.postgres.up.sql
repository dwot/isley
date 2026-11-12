-- Before 2.6 the incremental rollup refresh started its 25-hour window
-- mid-hour, so each run rewrote the oldest hour in the window from that
-- hour's last few minutes, and that partial row was then kept. Recompute
-- every hour that still has its readings. The earliest such hour is left
-- alone (pruning cut it part way through), and so are older hours: their
-- readings are gone, and the rollup is all that is left of them.
INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
SELECT sd.sensor_id, date_trunc('hour', sd.create_dt), MIN(sd.value), MAX(sd.value), AVG(sd.value), COUNT(*)
FROM sensor_data sd
WHERE sd.create_dt >= (SELECT date_trunc('hour', MIN(create_dt)) + INTERVAL '1 hour' FROM sensor_data)
GROUP BY sd.sensor_id, date_trunc('hour', sd.create_dt)
ON CONFLICT (sensor_id, bucket) DO UPDATE SET
    min_val      = EXCLUDED.min_val,
    max_val      = EXCLUDED.max_val,
    avg_val      = EXCLUDED.avg_val,
    sample_count = EXCLUDED.sample_count;
