-- 000746_hard_delete_permission.up.sql
-- Seeds the admin.hard_delete permission node (amended Rule 11): admin
-- ?force=true endpoints permanently remove already-soft-deleted rows for
-- holders of this node. Granted to super_admin and admin-group by default;
-- admins may grant/revoke it for other user groups via the RBAC admin UI.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('admin.hard_delete', 'Permanently delete already-soft-deleted rows via admin ?force=true endpoints')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'admin.hard_delete'
ON CONFLICT DO NOTHING;

COMMIT;
