-- 000521_credential_pathways_intel_provenance.up.sql
-- Extends credential_pathways with intelligence provenance columns so the
-- intelligence spine can promote credential pathway extractions into the
-- existing table. Mirrors the pattern applied to housing_guides (000445),
-- remittance_rates (000446), comparison_items (000447), and the 000469
-- tool-table extensions (wiki_pages, language_test_requirements,
-- tax_obligation_guides, community_organisations).
--
-- Ref: ADR-076 Amendment 1 (Approach B)
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;
ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE credential_pathways ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_credential_pathways_intel_record
    ON credential_pathways (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- System settings for the credential intelligence domain.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.credential.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.credential.freshness_decay_policy', '"slow"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
