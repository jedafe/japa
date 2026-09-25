-- 000263_remove_dead_settings_keys.up.sql
-- Removes 12 dead/duplicate system_settings keys that were identified during
-- the JEOS-vs-code gap audit. These keys had no Go consumers and duplicated
-- other mechanisms (feature_flags table, auth.email.verify_required, etc.).
-- Ref: .jeos/audit/backend-gap-report.md §3.5

DELETE FROM system_settings
WHERE key IN (
    -- Dead advanced/operational keys (process-startup concerns, env-only)
    'advanced.cache_ttl_seconds',
    'advanced.log_level',
    -- Dead feature flag duplicates (use feature_flags table instead)
    'features.advertising_enabled',
    'features.blog_enabled',
    'features.comparisons_enabled',
    'features.forum_enabled',
    'features.reviews_enabled',
    -- Dead security duplicates (use auth.email.verify_required + computed backoff)
    'security.lockout_duration_minutes',
    'security.require_email_verification',
    -- Dead SEO toggles (sitemap & robots.txt always served, JSON-LD via SSR)
    'seo.enable_json_ld',
    'seo.enable_robots_txt',
    'seo.enable_sitemap'
);
