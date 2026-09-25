-- Add visa_route custom field to user profiles so members can declare their
-- intended visa route. This field is used by the VisitorContext resolver
-- (ADR-064) and the journey-aware feed.
-- Ref: .jeos/sections/06-custom-field-builder-form-builder.md
--      docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md

-- 1. Ensure the User Profile Fields group exists (idempotent).
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

-- 2. Assign the group to the user_profile entity class (idempotent).
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT cfg.id, 'user_profile', NULL, 0
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'user_profile_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments cfa
      WHERE cfa.group_id = cfg.id AND cfa.entity_class = 'user_profile'
  );

-- 3. Create the visa_route field (select) with common visa route options.
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, show_in_frontend, position)
SELECT '00000000-0000-0000-0000-000000000042',
       cfg.id,
       '00000000-0000-0000-0000-000000000001',
       'Visa Route', 'visa_route', 'select',
       '[{"value":"student","label":"Student"},{"value":"work","label":"Work"},{"value":"business","label":"Business"},{"value":"tourist","label":"Tourist"},{"value":"spouse","label":"Spouse / Partner"},{"value":"family","label":"Family Reunion"},{"value":"investment","label":"Investment"},{"value":"transit","label":"Transit"},{"value":"other","label":"Other"}]'::jsonb,
       true, false, true, 2
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'user_profile_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf
      WHERE cf.group_id = cfg.id AND cf.key = 'visa_route'
  );
