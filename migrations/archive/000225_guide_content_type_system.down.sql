-- Reverse: remove guide content type system

-- Remove custom field assignments for guide categories
DELETE FROM custom_field_group_assignments
WHERE entity_class LIKE 'content_type:guide:%';

-- Remove seeded custom fields for guide groups
DELETE FROM custom_fields
WHERE group_id IN (
    SELECT id FROM custom_field_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN (
        'guide_visa_fields', 'guide_travel_routes_fields', 'guide_scholarship_fields',
        'guide_grants_fields', 'guide_admission_fields', 'guide_accommodation_fields',
        'guide_banking_fields', 'guide_work_career_fields',
        'guide_health_insurance_fields', 'guide_culture_lifestyle_fields'
      )
);

-- Remove seeded custom field groups
DELETE FROM custom_field_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN (
    'guide_visa_fields', 'guide_travel_routes_fields', 'guide_scholarship_fields',
    'guide_grants_fields', 'guide_admission_fields', 'guide_accommodation_fields',
    'guide_banking_fields', 'guide_work_career_fields',
    'guide_health_insurance_fields', 'guide_culture_lifestyle_fields'
  );

-- Remove guide category entity types
DELETE FROM custom_field_entity_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND value LIKE 'guide_category:%';

-- Remove guide permissions from role_permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'guide.create','guide.edit_own','guide.edit_any','guide.delete_own',
        'guide.delete_any','guide.publish','guide.feature'
    )
);

-- Remove guide permissions
DELETE FROM permissions WHERE key IN (
    'guide.create','guide.edit_own','guide.edit_any','guide.delete_own',
    'guide.delete_any','guide.publish','guide.feature'
);

-- Drop the guide_category column and index
DROP INDEX IF EXISTS idx_contents_guide_category;
ALTER TABLE contents DROP COLUMN IF EXISTS guide_category;
