-- The dedup pass (migrations 017-020) is not reversible: rows that
-- were merged into a canonical sensor cannot be un-merged. The down
-- migration is a no-op for the data step.
SELECT 1;
