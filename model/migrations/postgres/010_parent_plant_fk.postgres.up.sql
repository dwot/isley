-- Clean up invalid parent_plant_id values before adding the constraint.
-- Rows with 0 or referencing non-existent plants should be NULL (no parent).
UPDATE plant SET parent_plant_id = NULL
    WHERE parent_plant_id IS NOT NULL
      AND parent_plant_id NOT IN (SELECT id FROM plant);

-- Add foreign key constraint on parent_plant_id referencing plant(id)
ALTER TABLE plant ADD CONSTRAINT fk_plant_parent
    FOREIGN KEY (parent_plant_id) REFERENCES plant(id) ON DELETE SET NULL;
