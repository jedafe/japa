DROP INDEX IF EXISTS idx_user_ai_keys_key_hash;
ALTER TABLE user_ai_keys DROP COLUMN api_key_hash;
