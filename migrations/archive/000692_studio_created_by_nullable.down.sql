-- 000692_studio_created_by_nullable.down.sql
-- Renumbered from 000421 (duplicate version prefix). See the .up file.
-- 000421_studio_created_by_nullable.down.sql
-- Reverses 000421: restores NOT NULL on studio_jobs/batches.created_by.
-- NOTE: this will fail if any rows have NULL created_by (from GDPR erasure).

BEGIN;

ALTER TABLE studio_jobs    ALTER COLUMN created_by SET NOT NULL;
ALTER TABLE studio_batches ALTER COLUMN created_by SET NOT NULL;

COMMIT;
