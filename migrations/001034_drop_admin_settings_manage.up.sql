-- 001034_drop_admin_settings_manage.up.sql
-- Removes admin.settings.manage — a dead duplicate of admin.settings.
-- Both nodes share the description "Manage platform settings"; every admin
-- settings route (GET /admin/settings, PUT /admin/settings/{category}, plus
-- the onboarding/universities surfaces) is gated on admin.settings, and
-- admin.settings.manage is referenced by no route, handler, or tool.
-- The RBAC impersonation capability list is updated in the same commit.
-- Deleting the row cascades to role_permissions (FK CASCADE).

BEGIN;

DELETE FROM permissions WHERE key = 'admin.settings.manage';

COMMIT;
