-- 000201_credit_currency.down.sql

-- Remove RBAC grants
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'wallet.currency.manage', 'wallet.credit_pack.manage', 'wallet.purchases.view_any'
));

-- Remove permission nodes
DELETE FROM permissions WHERE key IN (
    'wallet.currency.manage', 'wallet.credit_pack.manage', 'wallet.purchases.view_any'
);

-- Remove system settings
DELETE FROM system_settings
WHERE key IN ('wallet.default_currency', 'wallet.credit_purchase_enabled');

-- Drop credit packs table
DROP TABLE IF EXISTS credit_packs;

-- Drop user_profiles column
ALTER TABLE user_profiles DROP COLUMN IF EXISTS preferred_currency;

-- Drop spaces column
ALTER TABLE spaces DROP COLUMN IF EXISTS currency_code;

-- Drop currencies table
DROP TABLE IF EXISTS currencies;
