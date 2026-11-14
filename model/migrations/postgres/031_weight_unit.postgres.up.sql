-- Harvest weights are stored in grams; weight_unit only changes how the web
-- UI displays them and which unit the plant forms offer first.
INSERT INTO settings (name, value)
SELECT 'weight_unit', 'g'
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE name = 'weight_unit');
