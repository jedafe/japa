-- 000447_intelligence_cost_of_living_extension.up.sql
-- Extends comparison_items with intelligence provenance columns so the
-- intelligence spine can promote cost-of-living extractions into the
-- existing table (ALTER approach per user decision). The existing
-- /cost-of-living API and tools page read enriched data with zero API
-- changes. The extracted cost breakdown is stored in the new
-- intel_cost_data jsonb column.
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS intel_cost_data jsonb;
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_comparison_items_intel_record
    ON comparison_items (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- System settings for the cost_of_living intelligence domain.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.cost_of_living.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.cost_of_living.freshness_decay_policy', '"normal"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
