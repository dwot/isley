ALTER TABLE strain ADD COLUMN ruderalis INTEGER NOT NULL DEFAULT 0;
UPDATE strain SET ruderalis = 0 WHERE ruderalis IS NULL;
