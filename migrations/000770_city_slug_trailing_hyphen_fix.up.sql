-- The first seed-geo-cities run wrote slugs ending in '-' for cities whose
-- GeoNames names end in punctuation (e.g. "Zurich (Kreis 10)" ->
-- 'zuerich-kreis-10-'): slugify emitted the dash but never trimmed it. The
-- seeder is fixed; this rewrites the 78 affected live rows.
-- Rows where the trimmed slug is already taken keep the seeder's '-cc'
-- collision suffix instead (one case: 'al-bayda' -> 'al-bayda-ly').
UPDATE public.spaces
SET slug = rtrim(slug, '-'), updated_at = now()
WHERE slug LIKE '%-' AND deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM public.spaces b
    WHERE b.deleted_at IS NULL
      AND b.id <> public.spaces.id
      AND b.slug = rtrim(public.spaces.slug, '-')
  );

UPDATE public.spaces
SET slug = rtrim(slug, '-') || '-' || lower(geo_code), updated_at = now()
WHERE slug LIKE '%-' AND deleted_at IS NULL;
