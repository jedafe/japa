-- 001062_sop_video_quota_tiers.down.sql

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'sop_generator.quota_standard.staff',
    'sop_generator.quota_standard.admin',
    'sop_generator.quota_standard.super_admin',
    'video.import.max_items_per_run.staff',
    'video.import.max_items_per_run.admin',
    'video.import.max_items_per_run.super_admin'
);

COMMIT;
