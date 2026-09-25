-- 000173_account_deletion_progress_step.down.sql
-- Reverts 000173: drops the progress_step column from account_deletion_requests.

BEGIN;

ALTER TABLE account_deletion_requests DROP COLUMN IF EXISTS progress_step;

COMMIT;
