-- Default to fem, most strains are fem these days
ALTER TABLE strain ADD COLUMN feminized BOOLEAN NOT NULL DEFAULT true;
