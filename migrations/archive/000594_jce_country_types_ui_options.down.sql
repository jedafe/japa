BEGIN;

UPDATE comparison_type_definitions
SET config = config - 'sort_options' - 'page_size_options'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration', 'residents_permit_requirements', 'business_registration');

COMMIT;
