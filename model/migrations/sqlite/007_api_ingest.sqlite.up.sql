-- Add default api_ingest_enabled setting (1 = enabled)
INSERT OR IGNORE INTO settings (name, value) VALUES ('api_ingest_enabled', '1');
