-- 000950_settings_dead_key_cleanup — removes system_settings rows that were
-- surfaced in Admin → Settings but never read by any service (dead controls),
-- plus the dead content.* aliases superseded by their canonical keys.
--
-- content.require_content_review flips to 'false': the setting was seeded
-- 'true' but never enforced, so 'true' was inert. Now that the publish path
-- honours the key, keeping 'true' would suddenly gate all publishing behind
-- the moderation queue — 'false' preserves the effective pre-wire behaviour
-- (review gate off unless an admin opts in).
UPDATE public.system_settings SET value = 'false' WHERE key = 'content.require_content_review';

DELETE FROM public.system_settings WHERE key IN (
    -- no consumer exists anywhere in the codebase
    'content.allow_anonymous_posts',
    'users.require_profile_completion',
    -- superseded by media.max_upload_mb (uploads share one media limit)
    'content.max_upload_size_mb',
    'users.max_avatar_size_mb',
    -- dead aliases; admin now writes the canonical comments.*/forum.* keys
    'content.comments.edit_window_minutes',
    'content.forum.edit_window_minutes',
    -- redundant variant: appearance.favicon_url already covers light theme
    'appearance.favicon_url_light'
);
