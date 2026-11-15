-- 001147_atproto_disabled_reason.down.sql

BEGIN;

ALTER TABLE atproto_accounts
    DROP CONSTRAINT IF EXISTS atproto_accounts_disabled_reason_check;

ALTER TABLE atproto_accounts
    DROP COLUMN IF EXISTS disabled_reason;

COMMIT;
