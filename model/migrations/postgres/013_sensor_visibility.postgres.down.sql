ALTER TABLE sensors ADD COLUMN show BOOLEAN NOT NULL DEFAULT TRUE;
UPDATE sensors SET show = CASE WHEN visibility = 'hide' THEN false ELSE true END;
ALTER TABLE sensors DROP COLUMN visibility;
