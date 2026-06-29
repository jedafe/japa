-- Section 47: Public API — add missing columns to api_keys
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS environment text NOT NULL DEFAULT 'live';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at timestamptz;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS notes text;
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);
