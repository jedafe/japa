-- 000463_studio_intel_record_id.up.sql
-- Approach B: Link Studio jobs to Intelligence extractions.
-- When intel_record_id is set, the Studio generation task loads the
-- extraction JSON and uses it as structured context for editorial generation.
-- Ref: ADR-076 Amendment 1, ADR-088.

BEGIN;

ALTER TABLE studio_jobs
    ADD COLUMN intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;

CREATE INDEX idx_studio_jobs_intel_record
    ON studio_jobs (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

COMMIT;
