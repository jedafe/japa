-- Phase 05: Reverse university seed migration.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05
-- Removes university seed data inserted by 000101_seed_universities.up.sql.
-- Only removes system-seeded universities (creator_user_id IS NULL, level=custom).

-- Remove parent relationships first
DELETE FROM space_parent_spaces
WHERE parent_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level = 'custom'
      AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
      AND creator_user_id IS NULL
);

-- Remove university spaces
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'custom'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND creator_user_id IS NULL;
