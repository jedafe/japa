-- Phase 05: Reverse geo seed migration.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05
-- Removes geo seed data inserted by 000100_seed_geo.up.sql.
-- Only removes system-seeded geo spaces (creator_user_id IS NULL, level IN country/state).

-- Remove parent relationships first (children before parents)
DELETE FROM space_parent_spaces
WHERE parent_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level IN ('country', 'state')
      AND creator_user_id IS NULL
);

-- Remove state-level spaces
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'state'
  AND creator_user_id IS NULL;

-- Remove country-level spaces
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'country'
  AND creator_user_id IS NULL;
