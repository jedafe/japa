-- 001038_region_wiki_categories.down.sql — remove the region category
-- template set; keeps the two 001037 categories (overview, countries).
BEGIN;

UPDATE wiki_type_categories SET label = 'Countries'
WHERE space_type_id = '00000000-0000-0000-0000-00000000007f' AND slug = 'countries';

DELETE FROM wiki_type_categories
WHERE space_type_id = '00000000-0000-0000-0000-00000000007f'
  AND slug IN ('cost-of-living','universities','attractions','embassies','banking','scholarships','other');

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN ('wiki_region_cost_of_living','wiki_region_universities','wiki_region_attractions',
                 'wiki_region_embassies','wiki_region_banking','wiki_region_scholarships','wiki_region_other');

DELETE FROM custom_field_groups
WHERE slug IN ('wiki_region_cost_of_living','wiki_region_universities','wiki_region_attractions',
               'wiki_region_embassies','wiki_region_banking','wiki_region_scholarships','wiki_region_other');

COMMIT;
