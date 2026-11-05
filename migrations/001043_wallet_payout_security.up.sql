-- 001043 — wallet payout security hardening (SEC-W series).
--   1. provider_ref index: the payout IPN webhook resolves withdrawals by
--      provider reference on every callback.
--   2. wallet_ledger_mutation_audit: every use of the ledger-immutability
--      bypass (wallet.allow_ledger_mutation) leaves an audit row. The
--      bypass exists for GDPR erasure — an attacker who sets the GUC can
--      also delete audit rows, so this is a tripwire for external log
--      shipping/alerting, not a hard control. True isolation requires the
--      erasure path to run under a dedicated role (ops decision).
--   3. Seed the new withdrawal security settings keys.

BEGIN;

-- ── 1. provider_ref lookup index ────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawals_provider_ref
    ON wallet_withdrawals (provider_ref)
    WHERE provider_ref IS NOT NULL;

-- ── 2. Mutation-bypass audit trail ──────────────────────────────────────
CREATE TABLE IF NOT EXISTS wallet_ledger_mutation_audit (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    table_name  text NOT NULL,
    operation   text NOT NULL,
    row_id      text,
    db_user text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
BEGIN
    IF current_setting('wallet.allow_ledger_mutation', true) = 'on' THEN
        -- Tripwire: record every bypass use so ops/alerting can spot
        -- mutation outside the GDPR erasure path.
        INSERT INTO wallet_ledger_mutation_audit (table_name, operation, row_id, db_user)
        VALUES (TG_TABLE_NAME, TG_OP,
                CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END,
                session_user);
        RAISE WARNING 'ledger mutation bypass used on %.% by %', TG_TABLE_NAME, TG_OP, session_user;
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION '% rows are immutable: % not allowed', TG_TABLE_NAME, TG_OP;
END;
$fn$;

-- ── 3. Withdrawal security settings ─────────────────────────────────────
-- wallet.daily_withdrawal_cap_credits: rolling-24h per-user credit cap
--   (0 = platform cap disabled; group auto-approve cap still applies).
-- wallet.withdrawal_method_hold_hours: payout methods younger than this
--   force manual approval on withdrawal (0 = hold disabled).
INSERT INTO system_settings (key, value)
VALUES
    ('wallet.daily_withdrawal_cap_credits', '0'::jsonb),
    ('wallet.withdrawal_method_hold_hours', '24'::jsonb),
    ('wallet.require_step_up', 'true'::jsonb),
    ('wallet.withdrawal_requires_verification', 'true'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
