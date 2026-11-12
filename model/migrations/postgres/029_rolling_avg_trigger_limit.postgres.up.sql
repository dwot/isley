-- SQLite-only change (its rolling-average trigger); Postgres already reads
-- the latest 16 readings off the index.
SELECT 1;
