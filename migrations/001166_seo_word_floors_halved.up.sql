-- §43 SEO quality-gate word floors halved:
--   content fallback 50 · blog/story 300 · list/wiki/comparison 250 ·
--   forum 15 · discussion 25 · guide 500.
BEGIN;

UPDATE public.system_settings SET value = '50'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.content_min_words';
UPDATE public.system_settings SET value = '300'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.blog_min_words';
UPDATE public.system_settings SET value = '300'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.story_min_words';
UPDATE public.system_settings SET value = '250'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.list_min_words';
UPDATE public.system_settings SET value = '250'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.wiki_min_words';
UPDATE public.system_settings SET value = '15'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.forum_min_words';
UPDATE public.system_settings SET value = '25'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.discussion_min_words';
UPDATE public.system_settings SET value = '500'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.guide_min_words';
UPDATE public.system_settings SET value = '250'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.comparison_min_words';

COMMIT;
