-- 000653_contents_source_provenance.up.sql
-- Track which intelligence source/record produced promoted or syndicated content.

BEGIN;

ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS intel_source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_contents_intel_source_id ON contents(intel_source_id) WHERE intel_source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_contents_intel_record_id ON contents(intel_record_id) WHERE intel_record_id IS NOT NULL;

COMMIT;
