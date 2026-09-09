-- 000643_video_enabled_public_setting.down.sql

UPDATE system_settings
SET is_public = false
WHERE key = 'video.enabled';
