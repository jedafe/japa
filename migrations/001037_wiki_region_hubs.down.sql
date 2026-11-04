-- 001037_wiki_region_hubs.down.sql — revert the regional wiki hubs.
-- Removes the parent links, hub spaces, type categories/groups, the
-- declared parent edge, the type config, the space type, and restores
-- geo type positions.

BEGIN;

DELETE FROM space_parent_spaces sps
USING spaces p
WHERE sps.parent_space_id = p.id AND p.level = 'region'
  AND p.space_type_id = '00000000-0000-0000-0000-00000000007f';

-- Wiki content seeded/materialized on the hub spaces (stub fan-out or
-- community submissions) must go before the spaces themselves.
DELETE FROM wiki_revisions wr
USING wiki_pages wp
WHERE wr.wiki_page_id = wp.id
  AND wp.space_id IN (
      SELECT id FROM spaces WHERE level = 'region'
        AND space_type_id = '00000000-0000-0000-0000-00000000007f'
  );

DELETE FROM wiki_pages
WHERE space_id IN (
    SELECT id FROM spaces WHERE level = 'region'
      AND space_type_id = '00000000-0000-0000-0000-00000000007f'
);

DELETE FROM spaces WHERE level = 'region'
  AND space_type_id = '00000000-0000-0000-0000-00000000007f';

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id AND g.slug IN ('wiki_region_overview', 'wiki_region_countries');

DELETE FROM wiki_type_categories WHERE space_type_id = '00000000-0000-0000-0000-00000000007f';
DELETE FROM custom_field_groups WHERE slug IN ('wiki_region_overview', 'wiki_region_countries');

DELETE FROM wiki_type_parents
WHERE space_type_id = '00000000-0000-0000-0000-000000000060'
  AND parent_space_type_id = '00000000-0000-0000-0000-00000000007f';

DELETE FROM wiki_type_configs WHERE space_type_id = '00000000-0000-0000-0000-00000000007f';
DELETE FROM space_types WHERE id = '00000000-0000-0000-0000-00000000007f';

COMMIT;
