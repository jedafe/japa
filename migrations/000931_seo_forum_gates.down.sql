BEGIN;
UPDATE public.system_settings SET value = '1'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.forum_min_replies';
UPDATE public.system_settings SET value = '1'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.question_min_answers';
COMMIT;
