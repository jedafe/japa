-- 000263_remove_dead_settings_keys.down.sql
-- Re-seeds the 12 dead/duplicate keys that were removed by the up migration.
-- This is provided for completeness; in practice these keys should not be
-- re-added as they have no consumers.

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'advanced.cache_ttl_seconds', '20'),
    ('00000000-0000-0000-0000-000000000001', 'advanced.log_level', '"info"'),
    ('00000000-0000-0000-0000-000000000001', 'features.advertising_enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'features.blog_enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'features.comparisons_enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'features.forum_enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'features.reviews_enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'security.lockout_duration_minutes', '15'),
    ('00000000-0000-0000-0000-000000000001', 'security.require_email_verification', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'seo.enable_json_ld', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'seo.enable_robots_txt', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'seo.enable_sitemap', 'true')
ON CONFLICT (tenant_id, key) DO NOTHING;
