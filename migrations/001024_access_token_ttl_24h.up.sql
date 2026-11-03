-- Raise the default access-token TTL from 15 minutes to 24 hours so sessions
-- read as "stay logged in until logout". The rotating jc_refresh cookie +
-- silent refresh already keep active sessions alive indefinitely; the longer
-- access token removes the frequent refresh round-trips that surfaced as
-- "token expires too quickly". Revocation safety is unchanged: JTI
-- revocation and the per-refresh user-status re-check still apply.
-- Ref: internal/auth/settings.go (DefaultAccessTokenTTL).
UPDATE system_settings
SET value = '"1440"'::jsonb, updated_at = now()
WHERE key = 'auth.session.access_token_ttl_minutes';
