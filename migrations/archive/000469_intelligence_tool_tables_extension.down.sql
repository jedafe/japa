-- 000469_intelligence_tool_tables_extension.down.sql
-- Reverts the intelligence provenance columns added by 000469.

BEGIN;

DROP INDEX IF EXISTS idx_community_organisations_intel_record;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS override_source;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS verification_status;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS guide_id;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_tax_obligation_guides_intel_record;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS override_source;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS verification_status;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS guide_id;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_language_test_requirements_intel_record;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS override_source;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS verification_status;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS guide_id;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_wiki_pages_intel_record;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS override_source;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS verification_status;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS guide_id;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS intel_record_id;

COMMIT;
