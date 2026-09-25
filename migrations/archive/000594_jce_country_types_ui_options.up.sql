-- Patch the three JCE country comparison types created in migration 000593 so
-- they carry the same default sort and page-size options as the other system
-- comparison types.

BEGIN;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'sort_options', to_jsonb(ARRAY['title', 'title_desc', 'members', 'newest']::text[]),
    'page_size_options', to_jsonb(ARRAY[12, 24, 48, 96]::int[])
)
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('bank_account_registration', 'residents_permit_requirements', 'business_registration');

COMMIT;
