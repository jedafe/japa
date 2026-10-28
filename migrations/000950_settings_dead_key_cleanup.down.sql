-- Restores the rows removed by 000950_settings_dead_key_cleanup with their
-- last seeded values.
UPDATE public.system_settings SET value = 'true' WHERE key = 'content.require_content_review';

INSERT INTO public.system_settings (key, value) VALUES
    ('content.allow_anonymous_posts', 'false'),
    ('content.max_upload_size_mb', '5'),
    ('content.comments.edit_window_minutes', '60'),
    ('content.forum.edit_window_minutes', '30'),
    ('users.require_profile_completion', 'false'),
    ('users.max_avatar_size_mb', '10'),
    ('appearance.favicon_url_light', '""')
ON CONFLICT DO NOTHING;
