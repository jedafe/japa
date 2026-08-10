-- BUG-011: Remove the orphaned `webhooks.manage` permission node.
-- The backend routes use `integrations.webhooks.manage` (server.go:3115).
-- The `webhooks.manage` node was only used by the admin sidebar (now fixed)
-- and is otherwise orphaned. Removing it eliminates RBAC ambiguity.
-- Ref: §12; .jeos/conventions/permission-key-naming.md

-- Remove role assignments for the orphaned node first (FK constraint).
DELETE FROM role_permission_assignments
WHERE permission_key = 'webhooks.manage';

-- Remove the permission node itself.
DELETE FROM permissions
WHERE key = 'webhooks.manage';
