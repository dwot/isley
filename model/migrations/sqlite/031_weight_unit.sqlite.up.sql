-- Harvest weights are stored in grams; weight_unit only changes how the web
-- UI displays them and which unit the plant forms offer first.
INSERT OR IGNORE INTO settings (name, value) VALUES ('weight_unit', 'g');
