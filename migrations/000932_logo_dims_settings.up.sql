-- Seed the appearance logo-dimension settings as PUBLIC so they reach
-- siteSettings (frontend width/height attrs for CLS). resolveLogoDims
-- populates them from the media row on every logo URL write.
BEGIN;
INSERT INTO public.system_settings (id, key, value, created_at, updated_at, is_public)
SELECT gen_random_uuid(), k.key, '0'::jsonb, now(), now(), true
FROM (VALUES
  ('appearance.logo_width'),
  ('appearance.logo_height'),
  ('appearance.logo_dark_width'),
  ('appearance.logo_dark_height')
) AS k(key)
WHERE NOT EXISTS (SELECT 1 FROM public.system_settings s WHERE s.key = k.key);
COMMIT;
