-- 001227_seed_digest_generation_settings.up.sql
-- Seeds the §84 Community Digest generation tuning keys so ops can see
-- and edit them in Control → Settings → Digest. The generator already
-- falls back to these compiled defaults when a key is unset; seeding
-- makes the effective values explicit and administrable.
--
-- Ref: japaconnect-engineering/sections/84-community-digest-widget.md
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('digest.cadence',            '"weekly"'),
    ('digest.top_threads_limit',  '5'),
    ('digest.new_members_limit',  '10'),
    ('digest.events_limit',       '5'),
    ('digest.recent_window_days', '7'),
    ('digest.story_window_days',  '30')
ON CONFLICT (key) DO NOTHING;

COMMIT;
