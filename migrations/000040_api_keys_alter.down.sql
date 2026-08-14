-- Down migration for 000040_api_keys_alter.sql
-- Drops the unique index and columns added to api_keys.
DROP INDEX IF EXISTS idx_api_keys_prefix;
ALTER TABLE api_keys DROP COLUMN IF EXISTS notes;
ALTER TABLE api_keys DROP COLUMN IF EXISTS revoked_by;
ALTER TABLE api_keys DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE api_keys DROP COLUMN IF EXISTS environment;
