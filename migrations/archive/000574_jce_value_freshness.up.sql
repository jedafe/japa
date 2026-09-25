-- 000574_jce_value_freshness.up.sql
-- JCE Sub-spec 4: track value provenance and freshness on comparison_values.
-- Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md

BEGIN;

ALTER TABLE comparison_values
    ADD COLUMN IF NOT EXISTS updated_at timestamptz,
    ADD COLUMN IF NOT EXISTS data_source_id uuid REFERENCES comparison_data_sources(id) ON DELETE SET NULL;

UPDATE comparison_values SET updated_at = created_at WHERE updated_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_values_data_source
    ON comparison_values(data_source_id);

COMMIT;
