-- 000615 down: restore the original field types and option lists.
-- The two divergent static lists are preserved verbatim.

-- Six-country list used by accommodation, banking, culture_lifestyle,
-- health_insurance, and work_career.
UPDATE custom_fields f
SET field_type = 'select',
    options = '["United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN ('guide_accommodation_fields', 'guide_banking_fields',
                 'guide_culture_lifestyle_fields', 'guide_health_insurance_fields',
                 'guide_work_career_fields')
  AND f.key = 'country';

-- Ten-country list used by admission (country) and visa (destination_country).
UPDATE custom_fields f
SET field_type = 'select',
    options = '["Nigeria","Ghana","Kenya","South Africa","United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND ((g.slug = 'guide_admission_fields' AND f.key = 'country')
    OR (g.slug = 'guide_visa_fields' AND f.key = 'destination_country'));

-- Relocation origin/destination were plain text fields with no options.
UPDATE custom_fields f
SET field_type = 'text',
    options = NULL
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_relocation_fields'
  AND f.key IN ('origin_country', 'destination_country');
