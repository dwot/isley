CREATE TABLE breeder (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL
);
-- Select distinct breeder from strain and insert into breeder table
INSERT INTO breeder (name)
SELECT DISTINCT breeder
FROM strain;
-- Add breeder_id to strain table
ALTER TABLE strain
ADD COLUMN breeder_id INTEGER;
-- Update breeder_id in strain table
UPDATE strain
SET breeder_id = (SELECT id
                  FROM breeder
                  WHERE breeder.name = strain.breeder);
-- Drop breeder column from strain table
ALTER TABLE strain
DROP COLUMN breeder;
-- END;