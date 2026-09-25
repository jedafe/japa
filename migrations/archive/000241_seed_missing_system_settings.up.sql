-- 000241_seed_missing_system_settings.up.sql
-- Seed the 25 system_settings keys that are referenced in application code
-- and documented in the system-settings-catalog but were never inserted by
-- migration 000099 (the original seed only covered the first 61 keys).
--
-- Without these rows the Settings service logs
--   "WARN settings: cache miss + DB miss key=…"
-- on every boot and on every periodic task that reads them, even though
-- the code falls back to hardcoded defaults. Seeding the rows makes the
-- values admin-editable via the Control Panel and silences the noise.
--
-- Idempotent: ON CONFLICT DO NOTHING preserves any tenant-specific override.
-- Seeded for the default tenant only; migration 000240 backfills copies to
-- other tenants. Values match the system-settings-catalog defaults.

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- Authentication (password policy gaps from 000099)
    ('00000000-0000-0000-0000-000000000001', 'auth.password.max_length',          '"128"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.password.require_number',      '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.password.require_symbol',      '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.password.check_breach',        '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.password.history_count',       '"5"'),

    -- Content sanitization
    ('00000000-0000-0000-0000-000000000001', 'content.sanitization.allowed_iframe_hosts', '"youtube.com,vimeo.com,codepen.io,jsfiddle.net,embed.ted.com"'),

    -- Feature flags
    ('00000000-0000-0000-0000-000000000001', 'feature.affiliate_program',         '"false"'),

    -- GDPR
    ('00000000-0000-0000-0000-000000000001', 'gdpr.policy_version',               '"1.0"'),

    -- Link preview
    ('00000000-0000-0000-0000-000000000001', 'linkpreview.fetch_timeout_seconds', '"10"'),

    -- Media
    ('00000000-0000-0000-0000-000000000001', 'media.video_thumbnail_time',        '"00:00:01"'),

    -- Organizations
    ('00000000-0000-0000-0000-000000000001', 'organizations.max_per_user',        '"5"'),

    -- Trending (Section 53 / ADR-032)
    ('00000000-0000-0000-0000-000000000001', 'trending.gravity',                  '"1.8"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.views',             '"1"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.votes',             '"10"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.comments',          '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.shares',            '"8"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.bookmarks',         '"3"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.space_member',      '"1"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.space_post',        '"2"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.space_recent',      '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.tag_content',       '"3"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.weight.tag_follower',      '"1"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.expiry.24h_hours',         '"2"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.expiry.7d_hours',          '"8"'),
    ('00000000-0000-0000-0000-000000000001', 'trending.expiry.30d_hours',         '"32"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
