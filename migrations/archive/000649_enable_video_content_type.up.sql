-- 000649_enable_video_content_type.up.sql
-- Flip video.enabled from false to true for all existing tenants.
-- This activates the §106 video content type for the current single-tenant
-- deployment and keeps it enabled by default for any tenants that were
-- seeded with the old default.
-- Ref: .jeos/sections/106-video-content-and-import.md

BEGIN;

UPDATE system_settings
SET value = 'true'::jsonb,
    updated_at = now()
WHERE key = 'video.enabled'
  AND value = 'false'::jsonb;

COMMIT;
