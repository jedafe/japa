-- 000649_enable_video_content_type.down.sql
-- Roll back the video.enabled flip.
-- Ref: .jeos/sections/106-video-content-and-import.md

BEGIN;

UPDATE system_settings
SET value = 'false'::jsonb,
    updated_at = now()
WHERE key = 'video.enabled'
  AND value = 'true'::jsonb;

COMMIT;
