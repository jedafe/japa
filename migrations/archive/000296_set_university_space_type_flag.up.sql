-- Set is_university = true for the seeded 'university' space type.
-- Migration 000161 added the column with DEFAULT false but never flagged
-- the University type. Every other space type correctly remains false.
UPDATE space_types
SET is_university = true, updated_at = now()
WHERE slug = 'university' AND tenant_id = '00000000-0000-0000-0000-000000000001';
