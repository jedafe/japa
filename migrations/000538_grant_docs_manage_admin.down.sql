-- 000538_grant_docs_manage_admin.down.sql
-- Revert DOC-001 grant.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'docs.manage'
)
AND role_id IN (
    SELECT id FROM user_roles WHERE slug IN ('admin', 'super_admin')
);

COMMIT;
