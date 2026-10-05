-- Reverses 000795: drops the attraction wiki type, its categories/fields, the
-- geo 'attractions' directory categories, and space_types.wiki_only.
-- custom_fields rows go away with their groups (group_id ON DELETE CASCADE);
-- wiki_type_categories must be deleted first (field_group_id is RESTRICT).

BEGIN;

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN (
      'wiki_attraction_overview', 'wiki_attraction_plan_your_visit',
      'wiki_country_attractions', 'wiki_state_attractions', 'wiki_city_attractions'
  );

DELETE FROM wiki_type_categories
WHERE (space_type_id = '00000000-0000-0000-0000-000000000071')
   OR (slug = 'attractions' AND space_type_id IN (
       '00000000-0000-0000-0000-000000000060',
       '00000000-0000-0000-0000-000000000061',
       '00000000-0000-0000-0000-000000000062'));

DELETE FROM custom_field_groups
WHERE slug IN (
    'wiki_attraction_overview', 'wiki_attraction_plan_your_visit',
    'wiki_country_attractions', 'wiki_state_attractions', 'wiki_city_attractions'
);

DELETE FROM wiki_type_parents WHERE space_type_id = '00000000-0000-0000-0000-000000000071';
DELETE FROM wiki_type_configs WHERE space_type_id = '00000000-0000-0000-0000-000000000071';
DELETE FROM space_types WHERE id = '00000000-0000-0000-0000-000000000071';

ALTER TABLE space_types DROP COLUMN wiki_only;

COMMIT;
