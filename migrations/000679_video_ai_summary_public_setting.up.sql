-- 000679_video_ai_summary_public_setting.up.sql
-- The /video/{slug} page reads video.ai_summary.enabled from GET
-- /site/settings/public to gate the "Generate AI draft" action (Rule 16) —
-- same precedent as 000643_video_enabled_public_setting.
-- Ref: japaconnect-engineering/sections/106-video-content-and-import.md

UPDATE system_settings
SET is_public = true
WHERE key = 'video.ai_summary.enabled';
