-- 000999 — reporting-role read access on credits_ledger.
-- Ref: conventions/wallet-ledger-split-plan.md ("DB grants" goal).
--
-- The monetary ledger now lives on its own table, so a read-only reporting
-- role can be scoped to it without ever touching points or the write path.
-- jc_reporting is NOLOGIN; membership is granted to whichever login role
-- the ops/reporting stack uses. wallet_ledger (points) stays app-only.
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'jc_reporting') THEN
        CREATE ROLE jc_reporting NOLOGIN;
    END IF;
END
$$;

REVOKE ALL ON credits_ledger FROM PUBLIC;
GRANT SELECT ON credits_ledger TO jc_reporting;

COMMIT;
