BEGIN;

UPDATE public.system_settings SET value = '100'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.blog_min_words';
UPDATE public.system_settings SET value = '200'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.guide_min_words';
UPDATE public.system_settings SET value = '50'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.comparison_min_words';
DELETE FROM public.system_settings WHERE key IN ('seo.sitemap.story_min_words', 'seo.sitemap.list_min_words', 'seo.sitemap.wiki_min_words');

COMMIT;
