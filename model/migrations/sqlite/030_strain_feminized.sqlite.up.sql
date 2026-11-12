-- Whether a strain's seeds are feminized (female plants only) or regular.
-- Existing and new strains default to feminized, as most seeds sold are.
-- An integer like autoflower, 1 or 0.
ALTER TABLE strain ADD COLUMN feminized INTEGER NOT NULL DEFAULT 1;
