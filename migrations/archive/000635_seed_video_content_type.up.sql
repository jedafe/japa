-- 000635_seed_video_content_type.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Seed the video JSON-LD template and the video.enabled feature flag.

INSERT INTO content_type_jsonld_templates (tenant_id, content_type, template, is_active, created_at, updated_at)
SELECT id, 'video', '{
    "@context": "https://schema.org",
    "@type": "VideoObject",
    "name": "{{title}}",
    "description": "{{description}}",
    "thumbnailUrl": "{{featured_image_url}}",
    "embedUrl": "{{source_url}}",
    "uploadDate": "{{published_at}}"
}'::jsonb, true, now(), now()
FROM tenants
ON CONFLICT (tenant_id, content_type) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value, created_at, updated_at)
SELECT id, 'video.enabled', 'false'::jsonb, now(), now()
FROM tenants
ON CONFLICT (tenant_id, key) DO NOTHING;
