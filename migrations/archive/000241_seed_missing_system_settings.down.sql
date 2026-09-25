-- 000241_seed_missing_system_settings.down.sql
-- Remove the 25 settings seeded by 000241. Only deletes rows whose value
-- still matches the seeded default, so any admin override is preserved.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'auth.password.max_length',
    'auth.password.require_number',
    'auth.password.require_symbol',
    'auth.password.check_breach',
    'auth.password.history_count',
    'content.sanitization.allowed_iframe_hosts',
    'feature.affiliate_program',
    'gdpr.policy_version',
    'linkpreview.fetch_timeout_seconds',
    'media.video_thumbnail_time',
    'organizations.max_per_user',
    'trending.gravity',
    'trending.weight.views',
    'trending.weight.votes',
    'trending.weight.comments',
    'trending.weight.shares',
    'trending.weight.bookmarks',
    'trending.weight.space_member',
    'trending.weight.space_post',
    'trending.weight.space_recent',
    'trending.weight.tag_content',
    'trending.weight.tag_follower',
    'trending.expiry.24h_hours',
    'trending.expiry.7d_hours',
    'trending.expiry.30d_hours'
  )
  AND value IN (
    '"128"', '"true"', '"false"', '"5"',
    '"youtube.com,vimeo.com,codepen.io,jsfiddle.net,embed.ted.com"',
    '"1.0"', '"10"', '"00:00:01"', '"1.8"',
    '"1"', '"10"', '"5"', '"8"', '"3"',
    '"2"', '"32"'
  );

COMMIT;
