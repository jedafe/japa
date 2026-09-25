-- Remove dead permission nodes that were superseded by more granular nodes.
-- forum.create: code uses forum.thread.create + forum.reply.create (server.go).
-- organizations.manage: code uses organizations.members.manage + organizations.invite (server.go).
-- These nodes were never checked by any RequirePermission call (AUDIT-BE-RBAC-007/008).

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('forum.create', 'organizations.manage')
);
DELETE FROM permissions WHERE key IN ('forum.create', 'organizations.manage');
