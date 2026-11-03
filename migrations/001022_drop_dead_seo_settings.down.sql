-- Restore the removed seo.* keys with their catalog defaults (empty values).
INSERT INTO system_settings (key, value) VALUES
    ('seo.meta_title_template', '""'::jsonb),
    ('seo.meta_description_template', '""'::jsonb),
    ('seo.og_image', '""'::jsonb),
    ('seo.twitter_handle', '""'::jsonb),
    ('seo.json_ld_type', '"Organization"'::jsonb),
    ('seo.google_analytics_id', '""'::jsonb),
    ('seo.google_tag_manager_id', '""'::jsonb)
ON CONFLICT (key) DO NOTHING;
