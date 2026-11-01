BEGIN;

REVOKE SELECT ON credits_ledger FROM jc_reporting;
DROP ROLE IF EXISTS jc_reporting;

COMMIT;
