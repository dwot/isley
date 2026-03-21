ALTER TABLE sensors ADD COLUMN visibility TEXT NOT NULL DEFAULT 'zone_plant';
UPDATE sensors SET visibility = CASE WHEN show = 1 THEN 'zone_plant' ELSE 'hide' END;
-- Note: SQLite < 3.35.0 does not support DROP COLUMN, so the old 'show'
-- column is left in place but is no longer read by application code.
