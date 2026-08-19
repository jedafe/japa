-- 000526_add_intel_sources_proposed_status.down.sql

BEGIN;

ALTER TABLE intel_sources
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS proposed_by,
    DROP COLUMN IF EXISTS proposed_at,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approved_at;

COMMIT;
