-- 000603_add_credit_withdrawal.down.sql
-- Revert credit withdrawal infrastructure.

BEGIN;

-- Remove system settings
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'wallet.withdrawal_default_currency',
    'wallet.daily_withdrawal_limit',
    'wallet.monthly_withdrawal_limit',
    'wallet.min_withdrawal_credits'
  );

-- Remove role grants for withdrawal permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('wallet.withdraw', 'wallet.withdraw.approve', 'wallet.withdraw.manage', 'wallet.payout_provider.manage')
);

-- Remove permission nodes
DELETE FROM permissions
WHERE key IN ('wallet.withdraw', 'wallet.withdraw.approve', 'wallet.withdraw.manage', 'wallet.payout_provider.manage');

-- Drop user group columns
ALTER TABLE user_groups
    DROP COLUMN IF EXISTS auto_withdrawal_limit,
    DROP COLUMN IF EXISTS requires_withdrawal_approval;

-- Drop withdrawal tables
DROP TABLE IF EXISTS wallet_withdrawals;
DROP TABLE IF EXISTS wallet_withdrawal_fee_tiers;
DROP TABLE IF EXISTS payout_providers;
DROP TABLE IF EXISTS wallet_withdrawal_methods;

-- Revert wallet ledger operations
ALTER TABLE wallet_ledger
    DROP CONSTRAINT wallet_ledger_operation_check,
    ADD CONSTRAINT wallet_ledger_operation_check
        CHECK (operation = ANY (ARRAY['earn', 'spend', 'purchase', 'convert', 'refund', 'clawback', 'admin_adjust']));

COMMIT;
