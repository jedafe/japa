-- 000430_intelligence_scholarships_backfill.down.sql
-- No-op: backfilled rows are real data now and are not deleted.
-- The scholarships table itself is dropped in 429d.
BEGIN;
-- Intentionally empty. See migration 429 down for table drop.
COMMIT;
