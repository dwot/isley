-- Add default api_ingest_enabled setting (1 = enabled) if not present
INSERT INTO settings (name, value)
SELECT 'api_ingest_enabled', '1'
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE name = 'api_ingest_enabled');
