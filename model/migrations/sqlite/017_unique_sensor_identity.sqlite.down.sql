-- Down migration only drops the unique index. The deduplication in the
-- up migration is not reversible: rows that were merged into a
-- canonical id cannot be un-merged.
DROP INDEX IF EXISTS idx_sensors_source_device_type;
