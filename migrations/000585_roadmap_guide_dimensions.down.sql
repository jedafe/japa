-- 000585_roadmap_guide_dimensions.down.sql
-- Remove the readiness dimension tag from the default roadmap guide contents.

BEGIN;

UPDATE contents
SET field_values = field_values - 'dimension'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN (
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
  );

COMMIT;
