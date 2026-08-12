-- BUG-011: Remove the orphaned `webhooks.manage` permission node.
-- The backend routes use `integrations.webhooks.manage` (server.go:3115).
-- The `webhooks.manage` node was only used by the admin sidebar (now fixed)
-- and is otherwise orphaned. Removing it eliminates RBAC ambiguity.
-- Ref: §12; .jeos/conventions/permission-key-naming.md
--
-- Note: the role↔permission join table is `role_permissions` (from
-- 000006_create_rbac_tables.sql), which maps role_id ↔ permission_id. There
-- is no `role_permission_assignments` table in the schema; an earlier draft
-- of this migration referenced that non-existent table, which errored on
-- databases that applied migrations transactionally (e.g. the testdb harness).
-- The DELETE below uses the correct table and is guarded so it is a no-op when
-- the `webhooks.manage` permission row has already been removed.

-- Remove role assignments for the orphaned node first (FK ON CASCADE would
-- also handle this when the permission row is deleted, but deleting explicitly
-- keeps the intent readable and survives any future FK change).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'webhooks.manage'
);

-- Remove the permission node itself.
DELETE FROM permissions
WHERE key = 'webhooks.manage';
