-- 000469_intelligence_tool_tables_extension.up.sql
-- Extends wiki_pages, language_test_requirements, tax_obligation_guides,
-- and community_organisations with intelligence provenance columns so the
-- intelligence spine can promote extractions into these existing tables.
-- Also adds guide_id (nullable FK to contents) for Approach B Studio
-- editorial linking (ADR-076 Amendment 1).
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

-- wiki_pages
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_wiki_pages_intel_record
    ON wiki_pages (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- language_test_requirements
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_language_test_requirements_intel_record
    ON language_test_requirements (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- tax_obligation_guides
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE tax_obligation_guides ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_tax_obligation_guides_intel_record
    ON tax_obligation_guides (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

-- community_organisations
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS confidence_overall real NOT NULL DEFAULT 0;
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified'
    CHECK (verification_status IN ('verified','needs_review','conflict','stale'));
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS freshness_score int4 NOT NULL DEFAULT 100;
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS override_source text NOT NULL DEFAULT 'ingestion'
    CHECK (override_source IN ('ingestion','editorial','mixed'));
ALTER TABLE community_organisations ADD COLUMN IF NOT EXISTS editorial_overrides jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_community_organisations_intel_record
    ON community_organisations (tenant_id, intel_record_id)
    WHERE intel_record_id IS NOT NULL;

COMMIT;
