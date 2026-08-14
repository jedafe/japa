-- Down migration for 000049_api_key_ip_allowlist.sql
-- Drops the api_key_ip_allowlist table used for API key IP restrictions.
DROP TABLE IF EXISTS api_key_ip_allowlist;
