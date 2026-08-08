UPDATE space_types
SET is_university = false, updated_at = now()
WHERE slug = 'university' AND tenant_id = '00000000-0000-0000-0000-000000000001';
