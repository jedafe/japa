-- Remove seo.* system_settings rows whose only existence was an admin
-- settings-UI control with no consumer. The controls were removed from
-- internal/admin/settings.go and internal/control/settings_catalog.go; the
-- stored values would otherwise linger as misleading "configuration" that
-- nothing reads.
-- Ref: japaconnect-engineering/sections/26-seo-performance.md.
DELETE FROM system_settings
WHERE key IN (
    'seo.meta_title_template',
    'seo.meta_description_template',
    'seo.og_image',
    'seo.twitter_handle',
    'seo.json_ld_type',
    'seo.google_analytics_id',
    'seo.google_tag_manager_id'
);
