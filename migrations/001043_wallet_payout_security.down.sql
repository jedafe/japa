BEGIN;

DELETE FROM system_settings WHERE key IN (
    'wallet.daily_withdrawal_cap_credits',
    'wallet.withdrawal_method_hold_hours'
);

-- Restore the pre-001043 reject_mutation (plain bypass, no audit).
CREATE OR REPLACE FUNCTION reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
BEGIN
    IF current_setting('wallet.allow_ledger_mutation', true) = 'on' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION '% rows are immutable: % not allowed', TG_TABLE_NAME, TG_OP;
END;
$fn$;

DROP TABLE IF EXISTS wallet_ledger_mutation_audit;
DROP INDEX IF EXISTS idx_wallet_withdrawals_provider_ref;

COMMIT;
