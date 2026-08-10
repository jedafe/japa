-- 000365_remove_orphaned_devices_permission_nodes.up.sql
-- Remove the orphaned `devices.manage` and `devices.admin` permission nodes.
--
-- These were seeded by migration 000261 to paper over a code drift: the
-- devices routes (§77) enforced `devices.manage`/`devices.admin` instead of
-- the spec-catalog nodes `push.receive`/`push.manage`. The code has been
-- corrected to use the catalog nodes (§77 / permission-key-naming.md), so
-- `devices.*` are now dead seed data with no route enforcing them.
--
-- The canonical nodes `push.manage` and `push.receive` were seeded by
-- migration 000257 and remain untouched.
--
-- Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
--      japaconnect-engineering/conventions/permission-key-naming.md (lines 170-171)

BEGIN;

-- Remove role_permissions grants for the orphaned nodes first.
-- (role_permissions.permission_id has ON DELETE CASCADE, but being explicit
-- avoids relying on a FK side-effect and makes the intent clear.)
DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions
  WHERE key IN ('devices.manage', 'devices.admin')
);

-- Remove the orphaned permission nodes themselves.
DELETE FROM permissions
WHERE key IN ('devices.manage', 'devices.admin');

COMMIT;
