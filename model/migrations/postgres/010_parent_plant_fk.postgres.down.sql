-- Remove the foreign key constraint on parent_plant_id
ALTER TABLE plant DROP CONSTRAINT IF EXISTS fk_plant_parent;
