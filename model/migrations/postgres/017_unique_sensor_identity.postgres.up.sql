-- Migration 017: dedup step 1 of 5 — repoint sensor_data rows.
-- See sqlite/017_unique_sensor_identity for full background.

UPDATE sensor_data sd
SET sensor_id = (
    SELECT MIN(s2.id) FROM sensors s2
    JOIN sensors s_self ON s_self.id = sd.sensor_id
    WHERE s2.source = s_self.source
      AND s2.device = s_self.device
      AND s2.type   = s_self.type
)
WHERE sensor_id IN (
    SELECT id FROM sensors WHERE id NOT IN (
        SELECT MIN(id) FROM sensors GROUP BY source, device, type
    )
);
