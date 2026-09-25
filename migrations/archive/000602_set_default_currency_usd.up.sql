-- 000602_set_default_currency_usd.up.sql
-- Set platform default currency to USD for new users and fallback resolution.
-- Ref: japaconnect-engineering/sections/58-credit-currency-system.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value)
VALUES ('00000000-0000-0000-0000-000000000001', 'wallet.default_currency', '"USD"')
ON CONFLICT (tenant_id, key)
DO UPDATE SET value = '"USD"', updated_at = now();

COMMIT;
