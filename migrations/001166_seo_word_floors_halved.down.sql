BEGIN;

UPDATE public.system_settings SET value = '100'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.content_min_words';
UPDATE public.system_settings SET value = '600'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.blog_min_words';
UPDATE public.system_settings SET value = '600'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.story_min_words';
UPDATE public.system_settings SET value = '500'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.list_min_words';
UPDATE public.system_settings SET value = '500'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.wiki_min_words';
UPDATE public.system_settings SET value = '30'::jsonb,   updated_at = now() WHERE key = 'seo.sitemap.forum_min_words';
UPDATE public.system_settings SET value = '50'::jsonb,   updated_at = now() WHERE key = 'seo.sitemap.discussion_min_words';
UPDATE public.system_settings SET value = '1000'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.guide_min_words';
UPDATE public.system_settings SET value = '500'::jsonb,  updated_at = now() WHERE key = 'seo.sitemap.comparison_min_words';

COMMIT;
