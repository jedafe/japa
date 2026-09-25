-- 000173_account_deletion_progress_step.up.sql
-- AUDIT-FRESH-GDPR-003: makes account erasure resumable. Adds a progress_step
-- counter to account_deletion_requests so a crashed Asynq job can resume from
-- the last completed step instead of re-running the whole flow from the top.
--
-- Step map (kept in sync with ProcessAccountDeletion in internal/gdpr/service.go):
--   0 = not started (pending)
--   1 = user row anonymized
--   2 = refresh tokens revoked
--   3 = valkey sessions deleted
--   4 = honcho AI context deleted
--   5 = hard delete performed (or retention_wait entered)
--   6 = marked completed
--
-- Zero-downtime: additive, NOT NULL with default 0, no backfill needed.

BEGIN;

ALTER TABLE account_deletion_requests
    ADD COLUMN IF NOT EXISTS progress_step int NOT NULL DEFAULT 0;

COMMIT;
