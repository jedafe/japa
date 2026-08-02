-- 000236_restore_comparison_any_permission_nodes.down.sql
-- Revert: remove the restored comparison.edit_any / delete_any nodes and
-- their super_admin grants.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('comparison.edit_any', 'comparison.delete_any')
);

DELETE FROM permissions
WHERE key IN ('comparison.edit_any', 'comparison.delete_any');
