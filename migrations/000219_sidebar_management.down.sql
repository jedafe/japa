-- 000219_sidebar_management.down.sql
-- Reverses the Sidebar Management System migration.
-- Ref: Section 59, ADR-066

BEGIN;

-- Remove RBAC grants + node.
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'sidebars.manage'
);
DELETE FROM permissions WHERE key = 'sidebars.manage';

-- Drop sidebar_blocks first (FK to sidebars).
DROP TABLE IF EXISTS sidebar_blocks;

-- Drop sidebars.
DROP TABLE IF EXISTS sidebars;

-- Note: the orphaned _sidebar_left / _sidebar_right page rows dropped in the
-- up migration are NOT re-seeded here. Re-running 000217's seed is out of scope
-- for a down migration; the deprecated approach is intentionally not restored.

COMMIT;
