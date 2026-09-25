-- Add the 'comparison' source and nested-comparison flags to the three JCE
-- country comparison types so the source selector matches other types.

BEGIN;

UPDATE comparison_type_definitions
SET config = config
    || jsonb_build_object(
        'item_ref_types', CASE
            WHEN config->'item_ref_types' IS NULL THEN to_jsonb(ARRAY['country', 'comparison']::text[])
            WHEN NOT (config->'item_ref_types') @> '["comparison"]'::jsonb THEN (config->'item_ref_types') || to_jsonb('comparison'::text)
            ELSE config->'item_ref_types'
        END,
        'allow_nested_comparisons', COALESCE(config->'allow_nested_comparisons', 'true'::jsonb),
        'allowed_nested_types', COALESCE(
            config->'allowed_nested_types',
            to_jsonb(ARRAY['business_registration', 'bank_account_registration', 'residents_permit_requirements']::text[])
        )
    )
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration', 'residents_permit_requirements', 'business_registration');

COMMIT;
