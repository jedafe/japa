-- 001205_grant_checklist_manage_to_admin.up.sql
-- Fix: §70/ADR-070 defines checklist.manage as the admin node, but the
-- original seed (000247) only granted it to super_admin — plain admin-group
-- members get 403 on /admin/checklist/*. Mirror of the visa_routes.manage
-- fix in 000426. Covers both admin-role slug variants across environments
-- ('admin' is the seeded slug, 'admin-group-role' is the group-role variant).

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'admin-group-role')
  AND p.key = 'checklist.manage'
ON CONFLICT DO NOTHING;

COMMIT;
