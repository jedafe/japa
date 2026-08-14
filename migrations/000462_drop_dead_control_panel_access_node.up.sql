-- 000462_drop_dead_control_panel_access_node.up.sql
-- Removes the dead control_panel.access permission node seeded by the
-- original 000184 migration. The backend (internal/server/server.go) and
-- JEOS sections/03-rbac.md use the dot-scoped name control.panel.access
-- (seeded by 000099/000125). The underscore variant was never enforced by
-- any route, so it is a dead duplicate. Migration 000184 has been corrected
-- to seed control.panel.access going forward; this migration cleans up
-- databases that already ran the old 000184.
-- Ref: japaconnect-engineering/sections/03-rbac.md

BEGIN;

-- 1. Remove any role_permissions grants referencing the dead node.
DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'control_panel.access'
);

-- 2. Remove the dead permission node itself.
DELETE FROM permissions WHERE key = 'control_panel.access';

COMMIT;
