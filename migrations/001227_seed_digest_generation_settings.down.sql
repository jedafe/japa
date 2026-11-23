-- 001227_seed_digest_generation_settings.down.sql
-- Removes the seeded §84 generation tuning keys (generator falls back to
-- compiled defaults).

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'digest.cadence',
    'digest.top_threads_limit',
    'digest.new_members_limit',
    'digest.events_limit',
    'digest.recent_window_days',
    'digest.story_window_days'
);

COMMIT;
