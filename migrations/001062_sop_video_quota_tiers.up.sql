-- 001062_sop_video_quota_tiers.up.sql
-- Staff-tier quota overrides for the remaining per-user quota tools:
-- sop_generator.quota_standard (monthly generations) and
-- video.import.max_items_per_run (per-import cap).

BEGIN;

INSERT INTO system_settings (key, value, updated_at) VALUES
    ('sop_generator.quota_standard.staff',       to_jsonb(25),  now()),
    ('sop_generator.quota_standard.admin',       to_jsonb(50),  now()),
    ('sop_generator.quota_standard.super_admin', to_jsonb(-1),  now()),
    ('video.import.max_items_per_run.staff',       to_jsonb(200), now()),
    ('video.import.max_items_per_run.admin',       to_jsonb(500), now()),
    ('video.import.max_items_per_run.super_admin', to_jsonb(-1),  now())
ON CONFLICT (key) DO NOTHING;

COMMIT;
