DROP INDEX IF EXISTS idx_atproto_oauth_tokens_request_uri;
ALTER TABLE atproto_oauth_tokens
    DROP COLUMN IF EXISTS request_uri,
    DROP COLUMN IF EXISTS refresh_rotated_at;
