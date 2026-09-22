-- 000690_seed_reports_rbac_nodes.down.sql
-- Renumbered from 000417 (duplicate version prefix). See the .up file.
-- 000417_seed_reports_rbac_nodes.down.sql
-- Reverses 000417: removes the reports.submit and reports.view_own grants and
-- permission nodes. Cascading role_permissions deletion is handled explicitly
-- to avoid relying on FK ON DELETE behavior that may differ across deployments.

BEGIN;

-- Remove role_permissions grants for the two nodes.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('reports.submit', 'reports.view_own')
);

-- Remove the permission nodes themselves.
DELETE FROM permissions WHERE key IN ('reports.submit', 'reports.view_own');

-- Remove the report.new notification type (Section 51).
DELETE FROM notification_types WHERE key = 'report.new';

COMMIT;
