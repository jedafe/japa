-- Reverse the university geo_code backfill. Sets geo_code back to NULL for
-- the 40 universities that were backfilled by the up migration.
UPDATE spaces SET geo_code = NULL
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND geo_code IN ('GB', 'CA', 'US', 'IE', 'DE', 'NL', 'ZA', 'NG');
