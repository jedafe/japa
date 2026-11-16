-- §43 SEO quality-gate word floors halved for long-prose types:
--   blog/story 300 · list/wiki/comparison 250 · guide 500.
-- content_min_words (generic fallback), forum_min_words, and
-- discussion_min_words are exempt — their floors were already small.
BEGIN;

UPDATE public.system_settings SET value = '300'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.blog_min_words';
UPDATE public.system_settings SET value = '300'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.story_min_words';
UPDATE public.system_settings SET value = '250'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.list_min_words';
UPDATE public.system_settings SET value = '250'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.wiki_min_words';
UPDATE public.system_settings SET value = '500'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.guide_min_words';
UPDATE public.system_settings SET value = '250'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.comparison_min_words';

COMMIT;
