-- Restore the previous 15-minute access-token TTL default.
UPDATE system_settings
SET value = '"15"'::jsonb, updated_at = now()
WHERE key = 'auth.session.access_token_ttl_minutes';
