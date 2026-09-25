-- 000366_add_ticket_rbac_nodes.down.sql
-- Reverse: remove the two ticket RBAC nodes added by the up migration.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions
  WHERE key IN ('tickets.view_all', 'tickets.comment.internal')
);

DELETE FROM permissions
WHERE key IN ('tickets.view_all', 'tickets.comment.internal');

COMMIT;
