-- Fix geo_code for the 2 universities whose slugs contain commas (the seed
-- slug generation in 000101 does not strip commas, so the backfill in 000264
-- missed them). Matched by name to avoid slug-matching issues.
UPDATE spaces SET geo_code = 'US'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND name = 'University of California, Berkeley';

UPDATE spaces SET geo_code = 'NG'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND name = 'University of Nigeria, Nsukka';
