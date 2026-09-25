-- 000653_contents_source_provenance.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_contents_intel_record_id;
DROP INDEX IF EXISTS idx_contents_intel_source_id;

ALTER TABLE contents
    DROP COLUMN IF EXISTS intel_record_id,
    DROP COLUMN IF EXISTS intel_source_id;

COMMIT;
