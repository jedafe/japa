-- 000679_video_ai_summary_public_setting.down.sql

UPDATE system_settings
SET is_public = false
WHERE key = 'video.ai_summary.enabled';
