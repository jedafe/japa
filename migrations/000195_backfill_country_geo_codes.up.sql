-- Backfill missing geo_code for country-level spaces that were seeded
-- before migration 000100_seed_geo and therefore skipped by its
-- ON CONFLICT (tenant_id, slug) DO NOTHING clause.
-- Ref: japaconnect-engineering/sections/25-ad-placement.md — target_geo
-- expects ISO 3166-1 alpha-2 codes; without geo_code the ad targeting
-- quick-pick falls back to slug, producing invalid codes like
-- "UNITED-STATES" instead of "US".

UPDATE spaces
SET geo_code = v.code
FROM (VALUES
    ('nigeria', 'NG'),
    ('united-kingdom', 'GB'),
    ('canada', 'CA'),
    ('united-states', 'US')
) AS v(slug, code)
WHERE spaces.level = 'country'
  AND spaces.slug = v.slug
  AND spaces.geo_code IS NULL;
