-- Remove the "Global" space type added in 000243.
DELETE FROM space_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'global';
