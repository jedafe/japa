-- Universities stop being a space TYPE: the seeded education type carries
-- them, and level='university' is what marks a row as a platform university
-- (primary, like geo levels). Member-created education spaces stay
-- level='custom' under the same type. Ref: §05 primary/member-created
-- amendment — member-created = level='custom' AND creator_user_id IS NOT NULL.
UPDATE spaces
SET level = 'university',
    space_type_id = (SELECT id FROM space_types WHERE slug = 'education')
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'university');

-- The education type takes over the university field group so universities
-- keep their 16 custom fields (initials, tuition_range, world_ranking, ...).
UPDATE space_types
SET field_group_id = (SELECT field_group_id FROM space_types WHERE slug = 'university')
WHERE slug = 'education';

-- wiki_type_parents FKs reference wiki_type_configs(space_type_id), so the
-- education type needs its own config row before parents/categories can be
-- re-pointed. Copy the university config verbatim.
INSERT INTO wiki_type_configs
 (space_type_id, enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator)
SELECT (SELECT id FROM space_types WHERE slug = 'education'),
 enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator
FROM wiki_type_configs
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'university');

-- wiki_type_categories cascades on the wtc delete — re-point the 10
-- university categories (Admissions, Tuition & Fees, ...) to education.
UPDATE wiki_type_categories
SET space_type_id = (SELECT id FROM space_types WHERE slug = 'education')
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'university');

UPDATE wiki_type_parents
SET space_type_id = (SELECT id FROM space_types WHERE slug = 'education')
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'university');

UPDATE wiki_type_parents
SET parent_space_type_id = (SELECT id FROM space_types WHERE slug = 'education')
WHERE parent_space_type_id = (SELECT id FROM space_types WHERE slug = 'university');

DELETE FROM wiki_type_configs
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'university');

-- The 'university' type is retired — members create 'education' spaces.
DELETE FROM space_types WHERE slug = 'university';
