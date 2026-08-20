-- Soft-delete the roadmap guide contents seeded by 000535 up.
-- assessment_results.roadmap_content normalization is a one-way data fix
-- and is intentionally not reversed.

UPDATE contents
SET deleted_at = now(),
    status = 'archived',
    updated_at = now()
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
