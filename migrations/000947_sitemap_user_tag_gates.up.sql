-- §43 sitemap quality gates for entity sitemaps:
--   seo.sitemap.user_min_posts  → minimum published content for a profile URL
--   seo.sitemap.tag_min_content → minimum content count for a tag URL
-- Both default to 1: profiles/tags with nothing on them are thin pages and
-- were previously advertised unconditionally.
BEGIN;

INSERT INTO public.system_settings (id, key, value, created_at, updated_at, is_public)
SELECT gen_random_uuid(), k.key, to_jsonb(k.val::integer), now(), now(), false
FROM (VALUES
  ('seo.sitemap.user_min_posts', '1'),
  ('seo.sitemap.tag_min_content', '1')
) AS k(key, val)
WHERE NOT EXISTS (SELECT 1 FROM public.system_settings s WHERE s.key = k.key);

COMMIT;
