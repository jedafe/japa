-- 000583_roadmap_guide_journey_metadata.up.sql
-- Tag the default roadmap guide contents with journey-stage hints so the
-- geo-aware readiness roadmap can rank them against a visitor's journey stage
-- (considering, planning, preparing, applied, etc.) in addition to any
-- target-country match.
--
-- Ref: docs/superpowers/plans/2026-08-31-geo-aware-readiness-roadmap.md

BEGIN;

UPDATE contents
SET
    journey_stage = CASE guide_keys.key
        WHEN 'guide-destination-research-guide' THEN 'considering'
        WHEN 'guide-visa-pathways-overview' THEN 'planning'
        WHEN 'guide-visa-document-checklist' THEN 'preparing'
        WHEN 'guide-credential-evaluation' THEN 'planning'
        WHEN 'guide-professional-certifications' THEN 'planning'
        WHEN 'track-language-proficiency' THEN 'planning'
        WHEN 'guide-connecting-with-diaspora' THEN 'relocated'
        ELSE contents.journey_stage
    END
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
    ]) AS key
) AS guide_keys
WHERE contents.slug = guide_keys.key
  AND contents.tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
