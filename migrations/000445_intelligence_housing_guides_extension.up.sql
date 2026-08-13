-- 000445_intelligence_housing_guides_extension.up.sql
-- Extends housing_guides with intelligence provenance columns so the
-- intelligence spine can promote housing/accommodation guide extractions
-- into the existing table (ALTER approach per user decision). The existing
-- /housing-guide API and tools page read enriched data with zero API
-- changes.
--
-- The housing promoter also creates a contents guide row
-- (guide_category='accommodation') so the guide content surface is fed —
-- the unified domain approach (housing_guides table + guide contents).
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

ALTER TABLE housing_guides ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE housing_guides ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE housing_guides ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE housing_guides ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE housing_guides ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE housing_guides ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_housing_guides_intel_record
    ON housing_guides (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- System settings for the housing intelligence domain.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.housing.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.housing.freshness_decay_policy', '"slow"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
