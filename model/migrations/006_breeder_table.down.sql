ALTER TABLE strain
ADD COLUMN breeder TEXT;
UPDATE strain
SET breeder = (SELECT name
               FROM breeder
               WHERE breeder.id = strain.breeder_id);
ALTER TABLE strain
DROP COLUMN breeder_id;
DROP TABLE breeder;
-- END;
