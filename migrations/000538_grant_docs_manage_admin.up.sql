-- 000538_grant_docs_manage_admin.up.sql
-- DOC-001: Grant the canonical docs.manage permission to admin and super-admin
-- roles. The backend and route catalog use docs.manage, while some earlier
-- grants only covered docs.admin. This migration is idempotent and safe for
-- tenants where the admin role does not yet exist.
-- Ref: .jeos/sections/44-documentation-system.md, .jeos/conventions/api-routes-catalog.md

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key = 'docs.manage'
WHERE ur.slug IN ('admin', 'super_admin')
ON CONFLICT DO NOTHING;

COMMIT;
