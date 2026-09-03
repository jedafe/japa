-- 000602_set_default_currency_usd.down.sql
-- Revert platform default currency to NGN.
-- Ref: japaconnect-engineering/sections/58-credit-currency-system.md

BEGIN;

UPDATE system_settings
SET value = '"NGN"',
    updated_at = now()
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'wallet.default_currency';

COMMIT;
