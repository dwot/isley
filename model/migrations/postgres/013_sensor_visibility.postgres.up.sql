ALTER TABLE sensors ADD COLUMN visibility TEXT NOT NULL DEFAULT 'zone_plant';
UPDATE sensors SET visibility = CASE WHEN show = true THEN 'zone_plant' ELSE 'hide' END;
ALTER TABLE sensors DROP COLUMN show;
