-- 001178_stablecoin_rbac.down.sql
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'stablecoin.wallet.view', 'treasury.view', 'treasury.sweep_approve', 'escrow.arbitrate'
    )
);
DELETE FROM permissions WHERE key IN (
    'stablecoin.wallet.view', 'treasury.view', 'treasury.sweep_approve', 'escrow.arbitrate'
);

COMMIT;
