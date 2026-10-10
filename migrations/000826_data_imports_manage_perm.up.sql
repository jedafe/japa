-- 000826_data_imports_manage_perm.up.sql — register the dedicated
-- data_imports.manage permission node used by the platform data-import
-- routes (GET/POST /admin/data-imports). Imports were previously gated by
-- the reused gdpr.data_exports.view node; a dedicated manage node lets
-- imports be granted independently of exports. Granted to the super_admin
-- roles following the same convention as moderation.appeal.review
-- (migration 000820); other roles can receive it via the RBAC admin UI.

INSERT INTO permissions (key, description) VALUES
    ('data_imports.manage', 'Manage platform data imports')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'data_imports.manage'
ON CONFLICT (role_id, permission_id) DO NOTHING;
