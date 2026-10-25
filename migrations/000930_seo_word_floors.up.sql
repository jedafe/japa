-- §43 SEO quality-gate word floors (product values):
--   blog/story 600 · list/wiki/comparison 500 · guide 1000.
-- Updates the seeded seo.sitemap.* rows and inserts the three new per-type
-- keys (story/list/wiki previously shared the generic content_min_words).
-- value is jsonb — stored as JSON numbers matching the existing seeded rows.
BEGIN;

UPDATE public.system_settings SET value = '600'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.blog_min_words';
UPDATE public.system_settings SET value = '1000'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.guide_min_words';
UPDATE public.system_settings SET value = '500'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.comparison_min_words';

INSERT INTO public.system_settings (id, key, value, created_at, updated_at, is_public)
SELECT gen_random_uuid(), k.key, to_jsonb(k.val::integer), now(), now(), false
FROM (VALUES
  ('seo.sitemap.story_min_words', '600'),
  ('seo.sitemap.list_min_words', '500'),
  ('seo.sitemap.wiki_min_words', '500')
) AS k(key, val)
WHERE NOT EXISTS (SELECT 1 FROM public.system_settings s WHERE s.key = k.key);

COMMIT;
