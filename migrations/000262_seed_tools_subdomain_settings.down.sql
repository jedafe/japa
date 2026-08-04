-- 000262_seed_tools_subdomain_settings.down.sql
-- Removes the 12 tools web surface subdomain/custom_domain settings seeded by
-- the up migration. The university.* keys (migration 000169) are NOT removed.

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'tools.subdomain', 'tools.custom_domain',
    'remittance.subdomain', 'remittance.custom_domain',
    'scholarship_finder.subdomain', 'scholarship_finder.custom_domain',
    'col_calculator.subdomain', 'col_calculator.custom_domain',
    'checklist.subdomain', 'checklist.custom_domain',
    'assessment.subdomain', 'assessment.custom_domain',
    'japabot.subdomain', 'japabot.custom_domain'
  );
