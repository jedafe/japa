-- 001071_audit_permission_grants.down.sql
BEGIN;

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id AND f.group_id = g.id
  AND g.slug IN (
      'geo_fields',
      'bank_account_registration_fields',
      'business_registration_fields',
      'residents_permit_requirements_fields',
      'wiki_remittance_overview',
      'wiki_languagetest_overview',
      'wiki_travel_overview'
  );

COMMIT;
