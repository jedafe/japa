-- The baseline 'abuja' city space was parented to the soft-deleted
-- 'federal-capital-territory' state (geo_code NG-FC); the live FCT state is
-- 'ng-abuja-federal-capital-territory' (geo_code FC). The seed-geo-cities run
-- could not resolve GeoNames admin1 'NG.11' ("FCT") to that live state, so it
-- created a second 'abuja-ng' row at country scope per the country fallback.
-- Reparent the original Abuja to the live FCT state (keeps the canonical id
-- and clean slug) and soft-delete the redundant country-scope copy, matching
-- the 000764 Nairobi dedup convention.
UPDATE public.spaces
SET parent_id = '42992cfb-b9b0-4bcf-97eb-7144ee435dab', updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000302' AND deleted_at IS NULL;

UPDATE public.space_parent_spaces
SET parent_space_id = '42992cfb-b9b0-4bcf-97eb-7144ee435dab'
WHERE space_id = '00000000-0000-0000-0000-000000000302';

UPDATE public.spaces
SET status = 'deleted', deleted_at = now()
WHERE slug = 'abuja-ng' AND deleted_at IS NULL;
