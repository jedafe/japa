BEGIN;

DELETE FROM public.system_settings WHERE key IN ('seo.sitemap.user_min_posts', 'seo.sitemap.tag_min_content');

COMMIT;
