-- Down migration for 000038_public_api.sql
-- Drops the api_keys table for external API access.
DROP TABLE IF EXISTS api_keys;
