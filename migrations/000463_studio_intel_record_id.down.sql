-- 000463_studio_intel_record_id.down.sql
-- Reverses: removes the intel_record_id column and its partial index from studio_jobs.
-- Ref: ADR-076 Amendment 1, ADR-088.

BEGIN;

DROP INDEX IF EXISTS idx_studio_jobs_intel_record;
ALTER TABLE studio_jobs DROP COLUMN IF EXISTS intel_record_id;

COMMIT;
