-- Phase 05: Reverse geo seed migration.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05
-- Removes geo seed data inserted by 000100_seed_geo.up.sql.
-- Only removes system-seeded geo spaces (level IN country/state for the
-- default tenant). The creator_user_id column was added by 000402 and is
-- not available when this down migration runs (000402.down runs first in
-- reverse order), so we identify seeded rows by tenant_id + level instead.

-- Remove parent relationships first (children before parents)
DELETE FROM space_parent_spaces
WHERE parent_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level IN ('country', 'state')
);

-- Remove state-level spaces
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'state';

-- Remove country-level spaces
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'country';
