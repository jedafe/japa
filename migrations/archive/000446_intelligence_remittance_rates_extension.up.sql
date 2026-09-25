-- 000446_intelligence_remittance_rates_extension.up.sql
-- Extends remittance_rates with intelligence provenance columns so the
-- intelligence spine can promote remittance rate extractions into the
-- existing table (ALTER approach per user decision). The existing
-- /remittance API and tools page read enriched data with zero API changes.
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

ALTER TABLE remittance_rates ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE remittance_rates ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE remittance_rates ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE remittance_rates ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE remittance_rates ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE remittance_rates ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_remittance_rates_intel_record
    ON remittance_rates (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- System settings for the remittance intelligence domain.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.remittance.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.remittance.freshness_decay_policy', '"fast"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
