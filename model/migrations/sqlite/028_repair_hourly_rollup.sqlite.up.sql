-- Before 2.6 the incremental rollup refresh started its 25-hour window
-- mid-hour, so each run rewrote the oldest hour in the window from that
-- hour's last few minutes, and that partial row was then kept. Recompute
-- every hour that still has its readings. The earliest such hour is left
-- alone (pruning cut it part way through), and so are older hours: their
-- readings are gone, and the rollup is all that is left of them.
INSERT OR REPLACE INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
SELECT sd.sensor_id, strftime('%Y-%m-%d %H:00:00', sd.create_dt), MIN(sd.value), MAX(sd.value), AVG(sd.value), COUNT(*)
FROM sensor_data sd
WHERE sd.create_dt >= (SELECT strftime('%Y-%m-%d %H:00:00', MIN(create_dt), '+1 hour') FROM sensor_data)
GROUP BY sd.sensor_id, strftime('%Y-%m-%d %H:00:00', sd.create_dt);
