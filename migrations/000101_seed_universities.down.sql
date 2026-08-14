-- Phase 05: Reverse university seed migration.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05
-- Removes university seed data inserted by 000101_seed_universities.up.sql.
-- Only removes system-seeded universities (level=custom, space_type=university
-- for the default tenant). The creator_user_id column was added by 000402 and
-- is not available when this down migration runs (000402.down runs first in
-- reverse order), so we identify seeded rows by tenant_id + level + space_type
-- instead.

-- Remove parent relationships first
DELETE FROM space_parent_spaces
WHERE parent_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level = 'custom'
      AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
);

-- Remove university spaces
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'custom'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university');
