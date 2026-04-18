ALTER TABLE activity ADD COLUMN is_watering BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE activity ADD COLUMN is_feeding BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE activity SET is_watering = TRUE WHERE name = 'Water';
UPDATE activity SET is_feeding = TRUE WHERE name = 'Feed';
