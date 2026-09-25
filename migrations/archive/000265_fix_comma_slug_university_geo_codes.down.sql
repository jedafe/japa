-- Reverse: set geo_code back to NULL for the 2 comma-slug universities.
UPDATE spaces SET geo_code = NULL
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND name IN ('University of California, Berkeley', 'University of Nigeria, Nsukka');
