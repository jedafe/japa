-- BUG-006: Seed the Japa Status custom field + 8 options on the user_profile
-- entity class. The japa_status field drives onboarding prompts, content
-- recommendations, space suggestions, and feed personalisation (ADR-064).
-- The 8 values are documented in .jeos/.ai/knowledge/user-personas.md.

-- 1. Create the "User Profile Fields" custom field group (if it doesn't exist).
INSERT INTO custom_field_groups (tenant_id, name, slug, description)
SELECT '00000000-0000-0000-0000-000000000001',
       'User Profile Fields', 'user_profile_fields',
       'Custom fields displayed on user profiles (Section 06).'
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'user_profile_fields'
);

-- 2. Assign the group to the user_profile entity class (if not already assigned).
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT cfg.id, 'user_profile', NULL, 0
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'user_profile_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments cfa
      WHERE cfa.group_id = cfg.id AND cfa.entity_class = 'user_profile'
  );

-- 3. Create the japa_status field (select type with 8 options).
INSERT INTO custom_fields (group_id, key, label, field_type, is_required, options, show_in_frontend, position)
SELECT cfg.id, 'japa_status', 'Japa Status', 'select', false,
       '[{"value":"newly_japa","label":"Newly Japa"},{"value":"settling_in","label":"Settling In"},{"value":"settled","label":"Settled"},{"value":"planning","label":"Planning"},{"value":"considering","label":"Considering"},{"value":"returned","label":"Returned"},{"value":"never_left","label":"Never Left"},{"value":"not_say","label":"Prefer Not to Say"}]'::jsonb,
       true, 0
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'user_profile_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf
      WHERE cf.group_id = cfg.id AND cf.key = 'japa_status'
  );
