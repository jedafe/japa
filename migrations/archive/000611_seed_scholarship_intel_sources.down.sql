-- 000611_seed_scholarship_intel_sources.down.sql
-- Removes the five scholarship intelligence sources seeded by the up migration.
-- Cascading foreign keys remove dependent intel_raw_documents, intel_records,
-- and (via scholarships.guide_id ON DELETE CASCADE) any linked contents rows.

BEGIN;

DELETE FROM intel_sources
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND source_url IN (
    'https://scholarships.grudged.io/api/scholarships?availability=open',
    'https://api.grants.gov/v1/api/search2',
    'https://www2.daad.de/deutschland/stipendium/datenbank/en/21148-scholarship-database/',
    'https://erasmus-plus.ec.europa.eu/opportunities/individuals/students/erasmus-mundus-joint-masters',
    'https://www.chevening.org/scholarships/'
  );

COMMIT;
