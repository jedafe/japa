-- 000604_drop_withdrawal_method_currency_fk.up.sql
-- wallet_withdrawal_methods.currency_code may hold crypto tickers (e.g. usdttrc20)
-- that are not in the fiat currencies table. Remove the foreign key.

BEGIN;

ALTER TABLE wallet_withdrawal_methods
    DROP CONSTRAINT IF EXISTS wallet_withdrawal_methods_currency_code_fkey;

COMMIT;
