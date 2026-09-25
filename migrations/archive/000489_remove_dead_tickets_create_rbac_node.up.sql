-- 000489_remove_dead_tickets_create_rbac_node.up.sql
-- Remove the `tickets.create` RBAC node. The ticket submission route enforces
-- `tickets.submit` (per Section 38), not `tickets.create`. The node was seeded
-- in 000099 and 000109 but never enforced, leaving a dead permission that
-- grants no actual access and confuses permission audits.
--
-- Ref: japaconnect-engineering/sections/38-ticket-system.md (RBAC section:
--   the canonical node is `tickets.submit`, not `tickets.create`)
--      japaconnect-engineering/conventions/permission-key-naming.md
--      .jeos/audit/tickets-audit.md BUG-009

BEGIN;

-- 1. Drop any role-permission grants for the dead node.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'tickets.create'
);

-- 2. Drop the permission node itself.
DELETE FROM permissions
WHERE key = 'tickets.create';

COMMIT;
