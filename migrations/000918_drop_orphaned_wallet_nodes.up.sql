-- 000910_drop_orphaned_wallet_nodes.up.sql
-- Removes the dead `wallet.view` / `wallet.convert` permission nodes. They were
-- granted to member/pro_member/business_member but enforced by no route — the
-- §28 routes enforce `wallet.points.balance` / `wallet.points.convert`
-- (granted by 000906). Orphaned grants mislead the RBAC admin UI: an admin can
-- grant a node that guards nothing.
-- Deleting the permission rows cascades to role_permissions (FK CASCADE),
-- removing the stale grants in the same step.

BEGIN;

DELETE FROM permissions
WHERE key IN ('wallet.view', 'wallet.convert');

COMMIT;
