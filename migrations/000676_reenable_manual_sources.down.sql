BEGIN;

-- 000676_reenable_manual_sources.down.sql
-- Re-enabling rows that the buggy circuit-breaker disabled is a one-way
-- repair; there is no meaningful rollback (an admin can re-disable a source
-- via PATCH enabled=false if needed). No action.

COMMIT;
