-- BUG-004: State-level geo spaces (level='state') were seeded without
-- setting parent_id for most countries. Only Nigeria (NG) and United
-- States (US) states had parent_id populated. This broke the
-- GetUserCurrentGeoSpace query (ADR-064a) which resolves the country
-- hierarchy via parent_id — GET /me/location returned null country
-- and country_code for any state-level location outside NG/US.
--
-- The state slugs follow the pattern '{cc}-{state_slug}' where {cc}
-- is the 2-letter ISO 3166-1 alpha-2 country code in lowercase.
-- We extract the prefix and join to country-level spaces by geo_code
-- (uppercase) to set parent_id.
--
-- Ref: ADR-064a (geo-awareness extension)
--      japaconnect-engineering/sections/05-spaces-geo.md

UPDATE spaces s
SET parent_id = c.id, updated_at = now()
FROM spaces c
WHERE s.level = 'state'
  AND s.parent_id IS NULL
  AND s.tenant_id = c.tenant_id
  AND s.deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND c.level = 'country'
  AND UPPER(split_part(s.slug, '-', 1)) = c.geo_code
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001';
