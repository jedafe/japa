-- 000635_seed_video_content_type.down.sql
DELETE FROM content_type_jsonld_templates WHERE content_type = 'video';
DELETE FROM system_settings WHERE key = 'video.enabled';
