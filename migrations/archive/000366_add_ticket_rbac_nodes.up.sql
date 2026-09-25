-- 000366_add_ticket_rbac_nodes.up.sql
-- Add the two Section 38 ticket permission nodes that were missing from the
-- seed: `tickets.view_all` (staff only — view all tickets) and
-- `tickets.comment.internal` (add internal notes). Without these nodes the
-- admin ticket routes cannot enforce the correct per-action permission and
-- fall back to the over-broad `tickets.admin` node.
--
-- Ref: japaconnect-engineering/sections/38-ticket-system.md (RBAC section)
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('tickets.view_all', 'Tickets View All'),
    ('tickets.comment.internal', 'Tickets Comment Internal')
ON CONFLICT (key) DO NOTHING;

-- Grant both new nodes to super_admin so the default admin can use them.
-- Note: the roles table is `user_roles` (from 000006_create_rbac_tables.sql);
-- an earlier draft referenced a non-existent `roles` table, which errored
-- on databases that apply migrations transactionally (e.g. the testdb harness).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin'
  AND p.key IN ('tickets.view_all', 'tickets.comment.internal')
ON CONFLICT DO NOTHING;

COMMIT;
