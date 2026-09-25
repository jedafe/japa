BEGIN;

UPDATE comparison_type_definitions
SET config = config
    - 'allow_nested_comparisons'
    - 'allowed_nested_types'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration', 'residents_permit_requirements', 'business_registration');

UPDATE comparison_type_definitions
SET config = jsonb_set(
    config,
    '{item_ref_types}',
    '["country"]'::jsonb
)
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration', 'residents_permit_requirements', 'business_registration');

COMMIT;
