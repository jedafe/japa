-- Add purpose and planned_travel_date custom fields to user profiles.
-- These capture the high-level travel purpose and planned departure date for
-- journey-aware recommendations.
-- Ref: .jeos/sections/06-custom-field-builder-form-builder.md
--      docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md

-- Ensure the User Profile Fields group exists.
INSERT INTO custom_field_groups (id, tenant_id, name, slug, description, is_system)
SELECT '00000000-0000-0000-0000-000000000041',
       '00000000-0000-0000-0000-000000000001',
       'User Profile Fields', 'user_profile_fields',
       'Custom fields displayed on user profiles (Section 06).', true
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'user_profile_fields'
);

-- purpose (select)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, show_in_frontend, position)
SELECT '00000000-0000-0000-0000-000000000043',
       cfg.id,
       '00000000-0000-0000-0000-000000000001',
       'Travel Purpose', 'purpose', 'select',
       '[{"value":"study","label":"Study"},{"value":"work","label":"Work"},{"value":"business","label":"Business"},{"value":"relocation","label":"Relocation"},{"value":"vacation","label":"Vacation"},{"value":"tourism","label":"Tourism"},{"value":"family","label":"Family"},{"value":"other","label":"Other"}]'::jsonb,
       true, false, true, 3
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'user_profile_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf
      WHERE cf.group_id = cfg.id AND cf.key = 'purpose'
  );

-- planned_travel_date (date)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, is_system, is_required, show_in_frontend, position)
SELECT '00000000-0000-0000-0000-000000000044',
       cfg.id,
       '00000000-0000-0000-0000-000000000001',
       'Planned Travel Date', 'planned_travel_date', 'date',
       true, false, true, 4
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'user_profile_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf
      WHERE cf.group_id = cfg.id AND cf.key = 'planned_travel_date'
  );
