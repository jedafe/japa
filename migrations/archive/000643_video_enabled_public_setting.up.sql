-- 000643_video_enabled_public_setting.up.sql
-- The /create composer and /studio read video.enabled from GET /site/settings/public
-- (Rule 16). Existing rows may have is_public = false from before the catalog change.
-- Ref: japaconnect-engineering/sections/106-video-content-and-import.md

UPDATE system_settings
SET is_public = true
WHERE key = 'video.enabled';
