-- 000604_drop_withdrawal_method_currency_fk.down.sql
-- Restore the foreign key on wallet_withdrawal_methods.currency_code.

BEGIN;

ALTER TABLE wallet_withdrawal_methods
    ADD CONSTRAINT wallet_withdrawal_methods_currency_code_fkey
    FOREIGN KEY (currency_code) REFERENCES currencies(code);

COMMIT;
