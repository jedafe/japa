-- 000359_seed_professional_profile_types.down.sql
-- Reverses the seed of the five system professional profile types.

DELETE FROM professional_profile_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('organization', 'agency', 'consultancy', 'brand', 'practice')
  AND is_system = true;
