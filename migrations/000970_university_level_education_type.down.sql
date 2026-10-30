-- Restore the 'university' space type (seeded fixed-id row) and move the
-- spaces + wiki config + categories + parents + field group back.
INSERT INTO space_types (id, name, slug, category, is_system, field_group_id)
VALUES ('00000000-0000-0000-0000-000000000064', 'University', 'university', 'education', true,
        '00000000-0000-0000-0000-000000000044')
ON CONFLICT (id) DO NOTHING;

UPDATE spaces
SET level = 'custom',
    space_type_id = '00000000-0000-0000-0000-000000000064'::uuid
WHERE level = 'university'
  AND space_type_id = (SELECT id FROM space_types WHERE slug = 'education');

-- Give education back its original (empty) field group.
UPDATE space_types
SET field_group_id = '00000000-0000-0000-0000-000000000043'::uuid
WHERE slug = 'education';

INSERT INTO wiki_type_configs
 (space_type_id, enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator)
SELECT '00000000-0000-0000-0000-000000000064'::uuid,
 enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator
FROM wiki_type_configs
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'education')
ON CONFLICT (space_type_id) DO NOTHING;

UPDATE wiki_type_categories
SET space_type_id = '00000000-0000-0000-0000-000000000064'::uuid
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'education');

UPDATE wiki_type_parents
SET space_type_id = '00000000-0000-0000-0000-000000000064'::uuid
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'education');

UPDATE wiki_type_parents
SET parent_space_type_id = '00000000-0000-0000-0000-000000000064'::uuid
WHERE parent_space_type_id = (SELECT id FROM space_types WHERE slug = 'education');

DELETE FROM wiki_type_configs
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'education');
