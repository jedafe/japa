-- 000593_jce_country_comparison_type_seeds.up.sql
-- Seed country-vs-country comparison types for Nigerian-migrant and business use cases.
-- These use the existing `country` space resolver with `region`/`subregion` scope chains.
--
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md

BEGIN;

-- 1. Custom field group for bank account registration.
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Bank Account Registration Fields',
    'bank_account_registration_fields',
    'Dimensions for comparing bank account registration requirements across countries',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT cfg.id,
       '00000000-0000-0000-0000-000000000001',
       field.label,
       field.key,
       field.field_type,
       false,
       true,
       true,
       true,
       true,
       true,
       field.position
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Minimum Deposit (USD)', 'minimum_deposit_usd', 'number', 0),
    ('Monthly Maintenance Fee (USD)', 'monthly_maintenance_fee_usd', 'number', 1),
    ('Accepts Nigerian ID', 'accepts_nigerian_id', 'boolean', 2),
    ('Requires Proof of Address', 'requires_proof_of_address', 'boolean', 3),
    ('Online Account Opening', 'online_account_opening', 'boolean', 4),
    ('USD Debit Card Available', 'usd_debit_card_available', 'boolean', 5),
    ('Required Documents', 'required_documents', 'text', 6),
    ('Typical Processing Time', 'typical_processing_time', 'text', 7)
) AS field(label, key, field_type, position)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'bank_account_registration_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- 2. Custom field group for residents permit requirements.
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Residents Permit Requirements Fields',
    'residents_permit_requirements_fields',
    'Dimensions for comparing resident permit requirements across countries',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT cfg.id,
       '00000000-0000-0000-0000-000000000001',
       field.label,
       field.key,
       field.field_type,
       false,
       true,
       true,
       true,
       true,
       true,
       field.position
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Eligibility Summary', 'eligibility_summary', 'text', 0),
    ('Minimum Stay Requirement', 'minimum_stay_requirement', 'text', 1),
    ('Application Fee (USD)', 'application_fee_usd', 'number', 2),
    ('Processing Time (Months)', 'processing_time_months', 'number', 3),
    ('Work Permit Included', 'work_permit_included', 'boolean', 4),
    ('Family Sponsorship', 'family_sponsorship', 'boolean', 5),
    ('Required Documents', 'required_documents', 'text', 6),
    ('Renewal Terms', 'renewal_terms', 'text', 7)
) AS field(label, key, field_type, position)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'residents_permit_requirements_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- 3. Custom field group for business registration.
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Business Registration Fields',
    'business_registration_fields',
    'Dimensions for comparing business registration requirements across countries',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT cfg.id,
       '00000000-0000-0000-0000-000000000001',
       field.label,
       field.key,
       field.field_type,
       false,
       true,
       true,
       true,
       true,
       true,
       field.position
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Business Entity Types', 'business_entity_types', 'text', 0),
    ('Registration Fee (USD)', 'registration_fee_usd', 'number', 1),
    ('Minimum Capital (USD)', 'minimum_capital_usd', 'number', 2),
    ('Local Director Required', 'local_director_required', 'boolean', 3),
    ('Foreign Ownership Allowed', 'foreign_ownership_allowed', 'boolean', 4),
    ('Tax ID Required', 'tax_id_required', 'boolean', 5),
    ('Processing Time (Days)', 'processing_time_days', 'number', 6),
    ('Required Documents', 'required_documents', 'text', 7)
) AS field(label, key, field_type, position)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'business_registration_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- 4. Comparison type definitions using the country space leaf.
INSERT INTO comparison_type_definitions (tenant_id, name, slug, description, field_group_id, item_label, is_system, config)
SELECT
    '00000000-0000-0000-0000-000000000001',
    type.name,
    type.slug,
    type.description,
    cfg.id,
    'Country',
    true,
    jsonb_build_object(
        'item_ref_types', ARRAY[type.leaf]::text[],
        'default_item_ref_type', type.leaf,
        'scope_chain', ARRAY['region', 'subregion'],
        'max_item_count', 5,
        'default_item_count', 2
    )
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Bank Account Registration', 'bank_account_registration', 'Compare bank account registration requirements for Nigerian migrants', 'country', 'bank_account_registration_fields'),
    ('Residents Permit Requirements', 'residents_permit_requirements', 'Compare resident permit requirements across countries', 'country', 'residents_permit_requirements_fields'),
    ('Business Registration', 'business_registration', 'Compare business registration requirements across countries', 'country', 'business_registration_fields')
) AS type(name, slug, description, leaf, field_group_slug)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = type.field_group_slug
ON CONFLICT (tenant_id, slug) DO NOTHING;

COMMIT;
