-- 000812_drop_dead_flag_keys.down.sql
-- The deleted rows were inert (no code path read them); restore them as
-- disabled rather than guessing their prior enabled state — safer than
-- silently enabling a feature that was off.
INSERT INTO feature_flags (key, enabled, updated_at)
VALUES ('newsletter.enabled', false, now()),
       ('referral.enabled', false, now())
ON CONFLICT (key) DO NOTHING;
