-- 000593_jce_country_comparison_type_seeds.down.sql
-- Revert the bank/residency/business country comparison type seeds.

BEGIN;

DELETE FROM comparison_type_definitions
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration', 'residents_permit_requirements', 'business_registration');

DELETE FROM custom_fields
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND group_id IN (
      SELECT id FROM custom_field_groups
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug IN ('bank_account_registration_fields', 'residents_permit_requirements_fields', 'business_registration_fields')
  );

DELETE FROM custom_field_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration_fields', 'residents_permit_requirements_fields', 'business_registration_fields');

COMMIT;
