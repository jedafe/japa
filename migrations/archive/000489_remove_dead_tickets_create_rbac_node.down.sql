-- 000489_remove_dead_tickets_create_rbac_node.down.sql
-- Reverse: re-seed the `tickets.create` permission node that 000489 removed.
-- This restores the pre-000489 state for rollback safety. The node is not
-- enforced by any route (the canonical node is `tickets.submit`), so
-- re-adding it does not restore any actual access.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('tickets.create', 'Create support tickets')
ON CONFLICT (key) DO NOTHING;

-- Re-grant to the member role to match the 000109 reconciliation state.
-- The member role is identified by slug 'member' in user_roles.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'member'
  AND p.key = 'tickets.create'
ON CONFLICT DO NOTHING;

COMMIT;
