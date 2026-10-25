-- §43 forum quality gates raised: threads need >= 2 replies; questions need
-- >= 2 answers (accepted/solved semantics) to index.
BEGIN;
UPDATE public.system_settings SET value = '2'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.forum_min_replies';
UPDATE public.system_settings SET value = '2'::jsonb, updated_at = now() WHERE key = 'seo.sitemap.question_min_answers';
COMMIT;
