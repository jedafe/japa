-- 000585_roadmap_guide_dimensions.up.sql
-- Tag the default roadmap guide contents with their readiness dimension in
-- field_values so the scoring worker can resolve a generic curated step to the
-- most specific published guide for the visitor's destination/stage/visa.
--
-- Ref: docs/superpowers/plans/2026-08-31-geo-aware-readiness-roadmap.md

BEGIN;

UPDATE contents
SET field_values = field_values || jsonb_build_object('dimension', guide_keys.dimension)
FROM (
    SELECT UNNEST(ARRAY[
        'guide-banking-finance-relocation',
        'tool-cost-of-living-calculator',
        'guide-visa-pathways-overview',
        'guide-visa-document-checklist',
        'guide-destination-research-guide',
        'guide-credential-evaluation',
        'guide-professional-certifications',
        'guide-connecting-with-diaspora',
        'guide-emotional-readiness-relocation',
        'track-language-proficiency'
    ]) AS key,
    UNNEST(ARRAY[
        'financial',
        'financial',
        'logistical',
        'logistical',
        'logistical',
        'professional',
        'professional',
        'personal',
        'personal',
        'language'
    ]) AS dimension
) AS guide_keys
WHERE contents.slug = guide_keys.key
  AND contents.tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
