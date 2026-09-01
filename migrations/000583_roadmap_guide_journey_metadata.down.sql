-- 000583_roadmap_guide_journey_metadata.down.sql
-- Clear the journey-stage hints added to the default roadmap guides.

BEGIN;

UPDATE contents
SET journey_stage = NULL
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
