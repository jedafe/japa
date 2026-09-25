-- 000236_restore_comparison_any_permission_nodes.up.sql
-- BUG-004 (revised): migration 000176 removed comparison.edit_any and
-- comparison.delete_any, claiming comparison.item.manage was the wired
-- equivalent. It is not — comparison.item.manage only gates item CRUD
-- (POST/PATCH/DELETE /comparisons/{id}/items), while the comparison
-- content row's edit/delete is gated by comparison.edit_own / .delete_own
-- (server.go L1845-1848). With the _any nodes gone, there is no permission
-- node that signals "admin-wide edit/delete of any comparison regardless
-- of authorship." This migration restores the two nodes and grants them
-- to super_admin so the admin panel can edit/delete any comparison.

BEGIN;

-- 1. Re-insert the removed permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('comparison.edit_any', 'Edit any comparison (admin/moderator)'),
    ('comparison.delete_any', 'Delete any comparison (admin/moderator)')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;

-- 2. Grant them to super_admin (which already has edit_own/delete_own, but
--    the _any nodes make the admin-wide intent explicit and let the admin
--    panel distinguish "edit any" from "edit own").
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('comparison.edit_any', 'comparison.delete_any')
ON CONFLICT DO NOTHING;

COMMIT;
