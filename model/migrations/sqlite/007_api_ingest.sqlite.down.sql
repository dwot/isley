-- Remove api_ingest_enabled setting
DELETE FROM settings WHERE name = 'api_ingest_enabled';
