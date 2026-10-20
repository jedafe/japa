-- 000918_drop_orphaned_wallet_nodes.down.sql
-- Restores the wallet.view / wallet.convert permission rows removed by 000918.
-- The role grants they held are not recoverable here (they cascaded on delete);
-- re-grant via the RBAC admin UI if ever needed.

BEGIN;

INSERT INTO permissions (id, key, description, created_at) VALUES
    ('ae3872ff-806f-4829-bc63-602eb9cd8fc6', 'wallet.view', 'View wallet balances', now()),
    ('55196519-a2cf-4984-b2a4-6682ca0bb423', 'wallet.convert', 'Convert points to credits', now())
ON CONFLICT (id) DO NOTHING;

COMMIT;
